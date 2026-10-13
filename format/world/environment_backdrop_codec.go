package world

import "github.com/karty-game/karty-sdk/internal/binwire"

// KWR2 keeps the existing solid trailer exactly when backdrop is omitted.
// Backdrop-only records use the nil solid sentinel (version 0, nil lists), then
// append one selector byte. Base KWR1 fields and bytes remain frozen.
func usesWorldExtensions(document *Document) bool {
	return document.SolidRenderers != nil || (document.Environment != nil && document.Environment.Backdrop != "")
}

func writeWorldExtensions(writer *binwire.Writer, document *Document) {
	solids := document.SolidRenderers
	if solids == nil {
		solids = &SolidRenderers{}
	}
	writeWorldSolidRenderers(writer, solids)
	if document.Environment != nil && document.Environment.Backdrop != "" {
		selector := uint8(0)
		switch document.Environment.Backdrop {
		case EnvironmentBackdropNone:
			selector = 1
		case EnvironmentBackdropMountains:
			selector = 2
		case EnvironmentBackdropCity:
			selector = 3
		}
		writer.U8(selector)
	}
}

func scanWorldExtensions(reader *binwire.Reader) error {
	scanWorldSolidRenderers(reader)
	if reader.Err() != nil {
		return reader.Err()
	}
	if reader.Remaining() > 1 {
		return ErrSyntax
	}
	if reader.Remaining() == 1 {
		selector := reader.U8()
		if selector < 1 || selector > 3 {
			return ErrEnvironment
		}
	}
	return nil
}

func readWorldExtensions(reader *binwire.Reader, document *Document) error {
	solids := new(SolidRenderers)
	readWorldSolidRenderers(reader, solids)
	if reader.Err() != nil {
		return reader.Err()
	}
	if solids.Version != 0 || solids.Prefabs != nil || solids.Actors != nil {
		document.SolidRenderers = solids
	}
	if reader.Remaining() == 1 {
		if document.Environment == nil {
			return ErrEnvironment
		}
		switch reader.U8() {
		case 1:
			document.Environment.Backdrop = EnvironmentBackdropNone
		case 2:
			document.Environment.Backdrop = EnvironmentBackdropMountains
		case 3:
			document.Environment.Backdrop = EnvironmentBackdropCity
		}
	}
	if !usesWorldExtensions(document) {
		return ErrSyntax
	}
	return nil
}
