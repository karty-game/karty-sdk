package textureatlas

import "strings"

type Entry struct {
	Name  string
	Bytes []byte
}

// ReadEntries validates the complete reserved namespace before exposing pages.
// Non-atlas assets such as fonts are ignored. Returned encoded pages are borrowed.
func ReadEntries(entries []Entry) (Bundle, bool, error) {
	reserved := make(map[string][]byte)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name, "@atlas/") {
			continue
		}
		if _, duplicate := reserved[entry.Name]; duplicate {
			return Bundle{}, false, ErrAtlas
		}
		reserved[entry.Name] = entry.Bytes
	}
	if len(reserved) == 0 {
		return Bundle{}, false, nil
	}
	layout, err := DecodeLayout(reserved[LayoutEntry])
	if err != nil {
		return Bundle{}, false, err
	}
	if len(reserved) != len(layout.Pages)+1 {
		return Bundle{}, false, ErrAtlas
	}
	bundle := Bundle{Layout: layout, Pages: make([][]byte, len(layout.Pages))}
	for index := range bundle.Pages {
		bundle.Pages[index] = reserved[PageEntryName(index)]
	}
	if err := bundle.Validate(); err != nil {
		return Bundle{}, false, err
	}
	return bundle, true, nil
}

func (bundle Bundle) Entries() ([]Entry, error) {
	if err := bundle.Validate(); err != nil {
		return nil, err
	}
	layout, err := EncodeLayout(bundle.Layout)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 1, len(bundle.Pages)+1)
	entries[0] = Entry{Name: LayoutEntry, Bytes: layout}
	for index, page := range bundle.Pages {
		entries = append(entries, Entry{Name: PageEntryName(index), Bytes: page})
	}
	return entries, nil
}
