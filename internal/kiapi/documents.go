package kiapi

import (
	"context"
	"fmt"
	"strings"

	commoncommands "kicadai/internal/kiapi/gen/common/commands"
	commontypes "kicadai/internal/kiapi/gen/common/types"
)

type DocumentType string

const (
	DocumentTypeUnknown   DocumentType = "unknown"
	DocumentTypeSchematic DocumentType = "schematic"
	DocumentTypeSymbol    DocumentType = "symbol"
	DocumentTypePCB       DocumentType = "pcb"
	DocumentTypeFootprint DocumentType = "footprint"
	DocumentTypeSheet     DocumentType = "drawing_sheet"
	DocumentTypeProject   DocumentType = "project"
)

type Document struct {
	Type          DocumentType `json:"type"`
	Identifier    string       `json:"identifier,omitempty"`
	SheetPath     string       `json:"sheet_path,omitempty"`
	BoardFilename string       `json:"board_filename,omitempty"`
	LibraryID     string       `json:"library_id,omitempty"`
	ProjectName   string       `json:"project_name,omitempty"`
	ProjectPath   string       `json:"project_path,omitempty"`
}

func (c *Client) GetOpenDocuments(ctx context.Context, documentType DocumentType) ([]Document, error) {
	var response commoncommands.GetOpenDocumentsResponse
	if err := c.Send(ctx, &commoncommands.GetOpenDocuments{
		Type: documentTypeProto(documentType),
	}, &response); err != nil {
		return nil, err
	}

	documents := make([]Document, 0, len(response.GetDocuments()))
	for _, specifier := range response.GetDocuments() {
		documents = append(documents, documentFromProto(specifier))
	}
	return documents, nil
}

func ParseDocumentType(value string) (DocumentType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all", "unknown":
		return DocumentTypeUnknown, nil
	case "schematic":
		return DocumentTypeSchematic, nil
	case "symbol":
		return DocumentTypeSymbol, nil
	case "pcb":
		return DocumentTypePCB, nil
	case "footprint":
		return DocumentTypeFootprint, nil
	case "drawing_sheet":
		return DocumentTypeSheet, nil
	case "project":
		return DocumentTypeProject, nil
	default:
		return DocumentTypeUnknown, fmt.Errorf("unsupported document type %q", value)
	}
}

func documentTypeProto(documentType DocumentType) commontypes.DocumentType {
	switch documentType {
	case DocumentTypeSchematic:
		return commontypes.DocumentType_DOCTYPE_SCHEMATIC
	case DocumentTypeSymbol:
		return commontypes.DocumentType_DOCTYPE_SYMBOL
	case DocumentTypePCB:
		return commontypes.DocumentType_DOCTYPE_PCB
	case DocumentTypeFootprint:
		return commontypes.DocumentType_DOCTYPE_FOOTPRINT
	case DocumentTypeSheet:
		return commontypes.DocumentType_DOCTYPE_DRAWING_SHEET
	case DocumentTypeProject:
		return commontypes.DocumentType_DOCTYPE_PROJECT
	default:
		return commontypes.DocumentType_DOCTYPE_UNKNOWN
	}
}

func documentTypeFromProto(documentType commontypes.DocumentType) DocumentType {
	switch documentType {
	case commontypes.DocumentType_DOCTYPE_SCHEMATIC:
		return DocumentTypeSchematic
	case commontypes.DocumentType_DOCTYPE_SYMBOL:
		return DocumentTypeSymbol
	case commontypes.DocumentType_DOCTYPE_PCB:
		return DocumentTypePCB
	case commontypes.DocumentType_DOCTYPE_FOOTPRINT:
		return DocumentTypeFootprint
	case commontypes.DocumentType_DOCTYPE_DRAWING_SHEET:
		return DocumentTypeSheet
	case commontypes.DocumentType_DOCTYPE_PROJECT:
		return DocumentTypeProject
	default:
		return DocumentTypeUnknown
	}
}

func documentFromProto(specifier *commontypes.DocumentSpecifier) Document {
	if specifier == nil {
		return Document{Type: DocumentTypeUnknown}
	}

	document := Document{Type: documentTypeFromProto(specifier.GetType())}
	if project := specifier.GetProject(); project != nil {
		document.ProjectName = project.GetName()
		document.ProjectPath = project.GetPath()
		document.Identifier = firstNonEmpty(project.GetPath(), project.GetName())
	}
	if sheetPath := specifier.GetSheetPath(); sheetPath != nil {
		document.SheetPath = strings.TrimSpace(sheetPath.GetPathHumanReadable())
		if document.SheetPath != "" {
			document.Identifier = document.SheetPath
		}
	}
	if boardFilename := strings.TrimSpace(specifier.GetBoardFilename()); boardFilename != "" {
		document.BoardFilename = boardFilename
		document.Identifier = boardFilename
	}
	if libID := specifier.GetLibId(); libID != nil {
		document.LibraryID = firstNonEmpty(joinLibraryID(libID), libID.GetEntryName(), libID.GetLibraryNickname())
		if document.LibraryID != "" {
			document.Identifier = document.LibraryID
		}
	}
	return document
}

func joinLibraryID(libID *commontypes.LibraryIdentifier) string {
	if libID.GetLibraryNickname() == "" || libID.GetEntryName() == "" {
		return ""
	}
	return libID.GetLibraryNickname() + ":" + libID.GetEntryName()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
