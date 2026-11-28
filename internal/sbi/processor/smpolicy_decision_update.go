package processor

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/free5gc/openapi/models"
)

// The response to an SM policy update request carries the policies the PCF updated, or an empty
// SmPolicyDecision when it does not update them (TS 29.512 clause 4.2.4.2). The PCF changes the
// SM policy decision it keeps in place, so the update is the difference between that decision
// before and after the request is handled.

// smPolicyDecisionSnapshot is the JSON encoding of each attribute of an SM policy decision.
type smPolicyDecisionSnapshot map[string]json.RawMessage

// removableMapAttributes are the SmPolicyDecision attributes that are maps of nullable values:
// an entry set to null in an update removes it (TS 29.512 clause 4.2.6).
var removableMapAttributes = func() map[string]bool {
	attributes := make(map[string]bool)
	decisionType := reflect.TypeOf(models.Pcf_SMPolCtrl_SmPolicyDecision{})
	for i := 0; i < decisionType.NumField(); i++ {
		field := decisionType.Field(i)
		if field.Type.Kind() != reflect.Map || field.Type.Elem().Kind() != reflect.Pointer {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		attributes[name] = true
	}
	return attributes
}()

// snapshotSmPolicyDecision takes a copy of decision to compare it once changed.
func snapshotSmPolicyDecision(
	decision *models.Pcf_SMPolCtrl_SmPolicyDecision,
) (smPolicyDecisionSnapshot, error) {
	encoded, err := json.Marshal(decision)
	if err != nil {
		return nil, err
	}
	var snapshot smPolicyDecisionSnapshot
	if err = json.Unmarshal(encoded, &snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// updatedSmPolicyDecision returns the policies of decision that changed since before was taken:
// the attributes that changed and, for the maps of policies, the entries that were added or
// changed, and the removed ones set to null. It is empty if no policy changed.
//
// An attribute other than a map that was removed cannot be signalled, as the data model of
// SmPolicyDecision does not encode null values for it; it is left out.
func updatedSmPolicyDecision(
	before smPolicyDecisionSnapshot, decision *models.Pcf_SMPolCtrl_SmPolicyDecision,
) (*models.Pcf_SMPolCtrl_SmPolicyDecision, error) {
	after, err := snapshotSmPolicyDecision(decision)
	if err != nil {
		return nil, err
	}
	updated := make(map[string]json.RawMessage)
	for name, value := range after {
		previous, existed := before[name]
		switch {
		case existed && bytes.Equal(previous, value):
		case existed && removableMapAttributes[name]:
			entries, entriesErr := updatedMapEntries(previous, value)
			if entriesErr != nil {
				return nil, entriesErr
			}
			if entries != nil {
				updated[name] = entries
			}
		default:
			updated[name] = value
		}
	}
	for name, previous := range before {
		if _, exists := after[name]; exists || !removableMapAttributes[name] {
			continue
		}
		entries, entriesErr := updatedMapEntries(previous, nil)
		if entriesErr != nil {
			return nil, entriesErr
		}
		if entries != nil {
			updated[name] = entries
		}
	}

	update := new(models.Pcf_SMPolCtrl_SmPolicyDecision)
	if len(updated) == 0 {
		return update, nil
	}
	encoded, err := json.Marshal(updated)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(encoded, update); err != nil {
		return nil, err
	}
	return update, nil
}

// updatedMapEntries returns the entries of the JSON object after that were added or changed
// since before, and the entries of before that were removed set to null; nil if there is none.
func updatedMapEntries(before, after json.RawMessage) (json.RawMessage, error) {
	var previous, current map[string]json.RawMessage
	if err := json.Unmarshal(before, &previous); err != nil {
		return nil, err
	}
	if after != nil {
		if err := json.Unmarshal(after, &current); err != nil {
			return nil, err
		}
	}
	entries := make(map[string]json.RawMessage)
	for key, value := range current {
		if old, existed := previous[key]; !existed || !bytes.Equal(old, value) {
			entries[key] = value
		}
	}
	for key := range previous {
		if _, exists := current[key]; !exists {
			entries[key] = json.RawMessage("null")
		}
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return json.Marshal(entries)
}
