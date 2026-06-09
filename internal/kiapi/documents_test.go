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
		"unknown":       DocumentTypeUnknown,
		"schematic":     DocumentTypeSchematic,
		"SCHEMATIC":     DocumentTypeSchematic,
		"symbol":        DocumentTypeSymbol,
		"pcb":           DocumentTypePCB,
		"footprint":     DocumentTypeFootprint,
		"drawing_sheet": DocumentTypeSheet,
		"project":       DocumentTypeProject,
	} {
		got, err := ParseDocumentType(raw)
		if err != nil {
			t.Errorf("ParseDocumentType(%q) returned error: %v", raw, err)
			continue
		}
		if got != want {
			t.Errorf("ParseDocumentType(%q) = %q, want %q", raw, got, want)
		}
	}

	if _, err := ParseDocumentType("bogus"); err == nil {
		t.Fatalf("ParseDocumentType returned nil error for bogus type")
	}
}

func TestDocumentTypeMappings(t *testing.T) {
	for documentType, wantProto := range map[DocumentType]commontypes.DocumentType{
		DocumentTypeUnknown:   commontypes.DocumentType_DOCTYPE_UNKNOWN,
		DocumentTypeSchematic: commontypes.DocumentType_DOCTYPE_SCHEMATIC,
		DocumentTypeSymbol:    commontypes.DocumentType_DOCTYPE_SYMBOL,
		DocumentTypePCB:       commontypes.DocumentType_DOCTYPE_PCB,
		DocumentTypeFootprint: commontypes.DocumentType_DOCTYPE_FOOTPRINT,
		DocumentTypeSheet:     commontypes.DocumentType_DOCTYPE_DRAWING_SHEET,
		DocumentTypeProject:   commontypes.DocumentType_DOCTYPE_PROJECT,
	} {
		if got := documentTypeProto(documentType); got != wantProto {
			t.Errorf("documentTypeProto(%q) = %s, want %s", documentType, got, wantProto)
		}
		if got := documentTypeFromProto(wantProto); got != documentType {
			t.Errorf("documentTypeFromProto(%s) = %q, want %q", wantProto, got, documentType)
		}
	}
	if got := documentTypeFromProto(commontypes.DocumentType(999)); got != DocumentTypeUnknown {
		t.Fatalf("documentTypeFromProto(999) = %q", got)
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

func TestDocumentFromProtoHandlesNilAndIdentifiers(t *testing.T) {
	if document := documentFromProto(nil); document.Type != DocumentTypeUnknown {
		t.Fatalf("nil document = %+v", document)
	}

	board := documentFromProto(&commontypes.DocumentSpecifier{
		Type: commontypes.DocumentType_DOCTYPE_PCB,
		Identifier: &commontypes.DocumentSpecifier_BoardFilename{
			BoardFilename: " board.kicad_pcb ",
		},
	})
	if board.Type != DocumentTypePCB || board.Identifier != "board.kicad_pcb" || board.BoardFilename != "board.kicad_pcb" {
		t.Fatalf("board document = %+v", board)
	}

	library := documentFromProto(&commontypes.DocumentSpecifier{
		Type: commontypes.DocumentType_DOCTYPE_SYMBOL,
		Identifier: &commontypes.DocumentSpecifier_LibId{
			LibId: &commontypes.LibraryIdentifier{
				LibraryNickname: "Device",
				EntryName:       "R",
			},
		},
	})
	if library.Identifier != "Device:R" || library.LibraryID != "Device:R" {
		t.Fatalf("library document = %+v", library)
	}

	entryOnly := documentFromProto(&commontypes.DocumentSpecifier{
		Type: commontypes.DocumentType_DOCTYPE_SYMBOL,
		Identifier: &commontypes.DocumentSpecifier_LibId{
			LibId: &commontypes.LibraryIdentifier{EntryName: "LED"},
		},
	})
	if entryOnly.Identifier != "LED" || entryOnly.LibraryID != "LED" {
		t.Fatalf("entry-only document = %+v", entryOnly)
	}

	nicknameOnly := documentFromProto(&commontypes.DocumentSpecifier{
		Type: commontypes.DocumentType_DOCTYPE_FOOTPRINT,
		Identifier: &commontypes.DocumentSpecifier_LibId{
			LibId: &commontypes.LibraryIdentifier{LibraryNickname: "Connector"},
		},
	})
	if nicknameOnly.Identifier != "Connector" || nicknameOnly.LibraryID != "Connector" {
		t.Fatalf("nickname-only document = %+v", nicknameOnly)
	}
}

func TestFirstNonEmptyTrimsValues(t *testing.T) {
	if got := firstNonEmpty(" ", "\tvalue\n", "later"); got != "value" {
		t.Fatalf("firstNonEmpty = %q", got)
	}
	if got := firstNonEmpty("", " "); got != "" {
		t.Fatalf("firstNonEmpty blank = %q", got)
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
