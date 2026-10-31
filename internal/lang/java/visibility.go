package java

func VisibilityScope(private bool) string {
	if private {
		return "file"
	}
	return "language"
}
