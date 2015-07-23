package email

func TstGetContentType(contentType string) (content, subtype string, param map[string]string, err error) {
	return getContentType(contentType)
}
