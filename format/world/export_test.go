package world

// EncodeUncheckedForTest builds semantic corruption fixtures with the production
// typed writer, without invoking public validation. It is absent from runtime.
func EncodeUncheckedForTest(document Document) ([]byte, error) { return encodeBinary(document) }
