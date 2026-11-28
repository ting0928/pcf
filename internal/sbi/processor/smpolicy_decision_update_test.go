package processor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/h2non/gock"
	"github.com/stretchr/testify/require"

	"github.com/free5gc/openapi"
	"github.com/free5gc/openapi/models"
)

func TestUpdatedSmPolicyDecision(t *testing.T) {
	decision := &models.Pcf_SMPolCtrl_SmPolicyDecision{
		PccRules: map[string]*models.Pcf_SMPolCtrl_PccRule{
			"kept":     {PccRuleId: "kept", Precedence: 10},
			"modified": {PccRuleId: "modified", Precedence: 20},
			"removed":  {PccRuleId: "removed", Precedence: 30},
		},
		QosDecs: map[string]*models.Pcf_SMPolCtrl_QosData{
			"qos-removed": {QosId: "qos-removed", Var5qi: 9},
		},
		PolicyCtrlReqTriggers: []models.Pcf_SMPolCtrl_PolicyControlRequestTrigger{
			models.Pcf_SMPolCtrl_PolicyControlRequestTrigger_PLMN_CH,
		},
		SuppFeat: "2F",
	}
	before, err := snapshotSmPolicyDecision(decision)
	require.NoError(t, err)

	decision.PccRules["modified"].Precedence = 21 // changed in place
	delete(decision.PccRules, "removed")
	decision.PccRules["added"] = &models.Pcf_SMPolCtrl_PccRule{PccRuleId: "added", Precedence: 40}
	decision.QosDecs = nil
	decision.PolicyCtrlReqTriggers = append(decision.PolicyCtrlReqTriggers,
		models.Pcf_SMPolCtrl_PolicyControlRequestTrigger_AN_INFO)

	updated, err := updatedSmPolicyDecision(before, decision)
	require.NoError(t, err)
	require.Equal(t, &models.Pcf_SMPolCtrl_SmPolicyDecision{
		PccRules: map[string]*models.Pcf_SMPolCtrl_PccRule{
			"modified": {PccRuleId: "modified", Precedence: 21},
			"removed":  nil,
			"added":    {PccRuleId: "added", Precedence: 40},
		},
		QosDecs: map[string]*models.Pcf_SMPolCtrl_QosData{"qos-removed": nil},
		PolicyCtrlReqTriggers: []models.Pcf_SMPolCtrl_PolicyControlRequestTrigger{
			models.Pcf_SMPolCtrl_PolicyControlRequestTrigger_PLMN_CH,
			models.Pcf_SMPolCtrl_PolicyControlRequestTrigger_AN_INFO,
		},
	}, updated)

	// The removed entries are encoded as null (TS 29.512 clause 4.2.6).
	encoded, err := json.Marshal(updated)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"removed":null`)
	require.Contains(t, string(encoded), `"qos-removed":null`)
}

// Without any change, the decision is empty (TS 29.512 clause 4.2.4.2, NOTE 5).
func TestUpdatedSmPolicyDecisionUnchanged(t *testing.T) {
	decision := &models.Pcf_SMPolCtrl_SmPolicyDecision{
		PccRules: map[string]*models.Pcf_SMPolCtrl_PccRule{"rule": {PccRuleId: "rule"}},
		SuppFeat: "2F",
	}
	before, err := snapshotSmPolicyDecision(decision)
	require.NoError(t, err)

	updated, err := updatedSmPolicyDecision(before, decision)
	require.NoError(t, err)
	require.Equal(t, &models.Pcf_SMPolCtrl_SmPolicyDecision{}, updated)
}

func updateSmPolicyWithRuleReport(t *testing.T, p *Processor, report models.Pcf_SMPolCtrl_RuleReport) string {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	p.HandleUpdateSmPolicyContextRequest(c, ruleReportPolicyID, models.Pcf_SMPolCtrl_SmPolicyUpdateContextData{
		RuleReports: []models.Pcf_SMPolCtrl_RuleReport{report},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	return w.Body.String()
}

// The PCC rules the PCF removes for a PCC rule report are provided removed in the response, and
// the PCC rules it keeps are not provided again (TS 29.512 clauses 4.2.4.2 and 4.2.4.15).
func TestSMPolicyUpdateRuleReportReturnsUpdatedPolicies(t *testing.T) {
	openapi.InterceptInnerHttp2Client(t, false)
	gock.Flush()
	t.Cleanup(gock.Off)
	p, _, _ := newRuleReportTestPCF(t, ruleReportPccRule, "PccRuleId-kept")
	interceptAfRequests(t)

	body := updateSmPolicyWithRuleReport(t, p, inactiveRuleReport(ruleReportPccRule))

	require.JSONEq(t, `{"pccRules":{"`+ruleReportPccRule+`":null}}`, body)
}

// A failed modification of a PCC rule the PCF keeps does not update the policies: the response
// carries an empty decision.
func TestSMPolicyUpdateRuleReportKeptReturnsEmptyDecision(t *testing.T) {
	openapi.InterceptInnerHttp2Client(t, false)
	gock.Flush()
	t.Cleanup(gock.Off)
	p, _, _ := newRuleReportTestPCF(t)
	interceptAfRequests(t)
	activeReport := inactiveRuleReport(ruleReportPccRule)
	activeReport.RuleStatus = models.Pcf_SMPolCtrl_RuleStatus_ACTIVE

	body := updateSmPolicyWithRuleReport(t, p, activeReport)

	require.JSONEq(t, `{}`, body)
}
