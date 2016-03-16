// Copyright (c) 2015 Monetas.
// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

func TstGetContentType(contentType string) (content, subtype string, param map[string]string, err error) {
	return getContentType(contentType)
}
