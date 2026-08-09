package mailer

func cloneBytes(data []byte) []byte {
	if data == nil {
		return nil
	}

	cloned := make([]byte, len(data))
	copy(cloned, data)
	return cloned
}

func cloneStrings(data []string) []string {
	if data == nil {
		return nil
	}

	cloned := make([]string, len(data))
	copy(cloned, data)
	return cloned
}
