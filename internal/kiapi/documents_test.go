package kiapi

import (
	"context"
	"testing"

	"kicadai/internal/ipc"
	"kicadai/internal/kiapi/gen/common"
	commoncommands "kicadai/internal/kiapi/gen/common/commands"
	commontypes "kicadai/internal/kiapi/gen/common/types"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestGetOpenDocumentsSendsTypeFilter(t *testing.T) {
	transport := &ipc.FakeTransport{}
	transport.QueueResponse(apiResponse(t, &commoncommands.GetOpenDocumentsResponse{
		Documents: []*commontypes.DocumentSpecifier{{
			Type: commontypes.DocumentType_DOCTYPE_SCHEMATIC,
			Identifier: &commontypes.DocumentSpecifier_SheetPath{
				SheetPath: &commontypes.SheetPath{PathHumanReadable: "/"},
			},
			Project: &commontypes.ProjectSpecifier{Name: "demo", Path: "/tmp/demo"},
		}},
	}))

	client, err := NewClient(context.Background(), testConfig(""), transport)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	documents, err := client.GetOpenDocuments(context.Background(), DocumentTypeSchematic)
	if err != nil {
		t.Fatalf("GetOpenDocuments returned error: %v", err)
	}
	if len(documents) != 1 {
		t.Fatalf("document count = %d", len(documents))
	}
	if documents[0].Type != DocumentTypeSchematic || documents[0].SheetPath != "/" {
		t.Fatalf("document = %+v", documents[0])
	}
	if documents[0].ProjectName != "demo" {
		t.Fatalf("project name = %q", documents[0].ProjectName)
	}

	request := sentRequest(t, transport)
	var command commoncommands.GetOpenDocuments
	if err := request.GetMessage().UnmarshalTo(&command); err != nil {
		t.Fatalf("unpacking GetOpenDocuments: %v", err)
	}
	if command.GetType() != commontypes.DocumentType_DOCTYPE_SCHEMATIC {
		t.Fatalf("type filter = %s", command.GetType())
	}
}

func TestParseDocumentType(t *testing.T) {
	for raw, want := range map[string]DocumentType{
		"":              DocumentTypeUnknown,
		"all":           DocumentTypeUnknown,
		"schematic":     DocumentTypeSchematic,
		"SCHEMATIC":     DocumentTypeSchematic,
		"pcb":           DocumentTypePCB,
		"drawing_sheet": DocumentTypeSheet,
	} {
		got, err := ParseDocumentType(raw)
		if err != nil {
			t.Fatalf("ParseDocumentType(%q) returned error: %v", raw, err)
		}
		if got != want {
			t.Fatalf("ParseDocumentType(%q) = %q, want %q", raw, got, want)
		}
	}

	if _, err := ParseDocumentType("bogus"); err == nil {
		t.Fatalf("ParseDocumentType returned nil error for bogus type")
	}
}

func TestDocumentFromProtoUsesProjectIdentifier(t *testing.T) {
	document := documentFromProto(&commontypes.DocumentSpecifier{
		Type:    commontypes.DocumentType_DOCTYPE_PROJECT,
		Project: &commontypes.ProjectSpecifier{Name: "demo", Path: "/tmp/demo"},
	})

	if document.Identifier != "/tmp/demo" {
		t.Fatalf("identifier = %q", document.Identifier)
	}
}

func apiResponse(t *testing.T, message proto.Message) []byte {
	t.Helper()

	packed, err := anypb.New(message)
	if err != nil {
		t.Fatalf("packing response: %v", err)
	}
	payload, err := proto.Marshal(&common.ApiResponse{
		Status:  &common.ApiResponseStatus{Status: common.ApiStatusCode_AS_OK},
		Message: packed,
	})
	if err != nil {
		t.Fatalf("marshaling response: %v", err)
	}
	return payload
}
