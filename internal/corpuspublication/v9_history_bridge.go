package corpuspublication

// OpenHeldOutCommitmentsV8 authenticates and opens the frozen V8 record set
// but returns only the commitment metadata needed to retire that corpus from
// V9 authoring. Requirement plaintext is cleared before this function returns
// and is never exposed through its API. Keeping this V9-owned adapter separate
// preserves every byte covered by the frozen V8 publisher manifest.
func OpenHeldOutCommitmentsV8(key []byte, manifest ManifestV8, ciphertext []byte) ([]EntryV8, error) {
	cases, err := OpenHeldOutV8(key, manifest, ciphertext)
	if err != nil {
		return nil, err
	}
	entries := make([]EntryV8, len(cases))
	for index := range cases {
		entries[index] = cases[index].Entry
		clear(cases[index].Source)
		cases[index].Source = nil
	}
	return entries, nil
}
