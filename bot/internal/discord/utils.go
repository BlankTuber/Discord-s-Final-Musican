package discord

// stringPtr returns a pointer to the given string
func stringPtr(s string) *string {
	return &s
}

// floatPtr returns a pointer to the given float64
func floatPtr(f float64) *float64 {
	return &f
}