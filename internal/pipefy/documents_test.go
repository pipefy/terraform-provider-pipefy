// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pipefy

import (
	"os"
	"strings"
	"testing"
)

// sdkDocuments is every document this package sends, by operation name. Each
// entity adds its operations here when it is migrated.
var sdkDocuments = map[string]string{
	"CreateLabel_tf":   createLabelMutation,
	"GetPipeLabels_tf": getPipeLabelsQuery,
	"UpdateLabel_tf":   updateLabelMutation,
	"DeleteLabel_tf":   deleteLabelMutation,

	"CreatePipe_tf":    createPipeMutation,
	"GetPipe_tf":       getPipeQuery,
	"GetPipePhases_tf": getPipePhasesQuery,
	"UpdatePipe_tf":    updatePipeMutation,
	"DeletePipe_tf":    deletePipeMutation,
	"GetPipeUuid_tf":   getPipeUUIDQuery,

	"CreatePhase_tf":    createPhaseMutation,
	"GetPhase_tf":       getPhaseQuery,
	"UpdatePhase_tf":    updatePhaseMutation,
	"DeletePhase_tf":    deletePhaseMutation,
	"GetPhaseRepoId_tf": getPhaseRepoIDQuery,

	"CreatePhaseField_tf": createPhaseFieldMutation,
	"GetPhaseFields_tf":   getPhaseFieldsQuery,
	"UpdatePhaseField_tf": updatePhaseFieldMutation,
	"DeletePhaseField_tf": deletePhaseFieldMutation,

	"CreateTable_tf": createTableMutation,
	"GetTable_tf":    getTableQuery,
	"UpdateTable_tf": updateTableMutation,
	"DeleteTable_tf": deleteTableMutation,

	"CreateTableField_tf": createTableFieldMutation,
	"GetTableFields_tf":   getTableFieldsQuery,
	"UpdateTableField_tf": updateTableFieldMutation,
	"DeleteTableField_tf": deleteTableFieldMutation,

	"CreateWebhook_tf":   createWebhookMutation,
	"GetPipeWebhooks_tf": getPipeWebhooksQuery,
	"UpdateWebhook_tf":   updateWebhookMutation,
	"DeleteWebhook_tf":   deleteWebhookMutation,

	"CreatePipeRelation_tf": createPipeRelationMutation,
	"GetPipeRelations_tf":   getPipeRelationsQuery,
	"UpdatePipeRelation_tf": updatePipeRelationMutation,
	"DeletePipeRelation_tf": deletePipeRelationMutation,

	"CreateAutomation_tf": createAutomationMutation,
	"GetAutomation_tf":    getAutomationQuery,
	"UpdateAutomation_tf": updateAutomationMutation,
	"DeleteAutomation_tf": deleteAutomationMutation,

	"CreateFieldCondition_tf": createFieldConditionMutation,
	"GetFieldCondition_tf":    getFieldConditionQuery,
	"UpdateFieldCondition_tf": updateFieldConditionMutation,
	"DeleteFieldCondition_tf": deleteFieldConditionMutation,

	"CreateAiAgent_tf":       createAIAgentMutation,
	"GetAiAgent_tf":          getAIAgentQuery,
	"UpdateAiAgent_tf":       updateAIAgentMutation,
	"UpdateAiAgentStatus_tf": updateAIAgentStatusMutation,
	"DeleteAiAgent_tf":       deleteAIAgentMutation,
}

func loadGoldenDocuments(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("testdata/documents.golden")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	golden := map[string]string{}
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		name, document, ok := strings.Cut(line, "\t")
		if !ok {
			t.Fatalf("malformed fixture line: %q", line)
		}
		golden[name] = document
	}
	return golden
}

// TestDocumentsMatchGolden fails when a migrated document differs from what the
// provider sent before the move. A deliberate change updates the fixture in the
// same commit.
func TestDocumentsMatchGolden(t *testing.T) {
	golden := loadGoldenDocuments(t)
	for name, got := range sdkDocuments {
		want, ok := golden[name]
		if !ok {
			t.Errorf("%s: not in the fixture", name)
			continue
		}
		if got != want {
			t.Errorf("%s: document changed\n got: %s\nwant: %s", name, got, want)
		}
	}
}

// TestGoldenFullyMigrated fails while any captured operation still lives outside
// this package.
func TestGoldenFullyMigrated(t *testing.T) {
	for name := range loadGoldenDocuments(t) {
		if _, ok := sdkDocuments[name]; !ok {
			t.Errorf("%s is in the fixture but not in sdkDocuments", name)
		}
	}
}

// TestDocumentNamesMatchOperations fails when a document is filed under an
// operation name its text does not declare, which would let a rename slip past
// the golden comparison.
func TestDocumentNamesMatchOperations(t *testing.T) {
	for name, document := range sdkDocuments {
		if !strings.Contains(document, " "+name+"(") {
			t.Errorf("%s: document does not declare that operation name: %s", name, document)
		}
	}
}
