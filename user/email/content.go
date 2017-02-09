// Copyright 2016 Daniel Krawisz.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package email

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/mailhog/data"
)

var (
	contentTypeToken = `[^ \(\)<>@,;:\\\"/\[\]\?\.=[:cntrl:]]+`

	contentTypeType = fmt.Sprintf("(?:application|audio|image|message|multipart|text|video|x\\-%s)", contentTypeToken)

	contentTypeValue = fmt.Sprintf("(?:\\\"[^\\\"]*\\\"|%s)", contentTypeToken)

	contentTypeRegex = regexp.MustCompile(
		fmt.Sprintf("^\\s?(%s)\\s?/\\s?(%s)(?:\\s?;\\s?(%s)=(%s))*\\s?$",
			contentTypeType, contentTypeToken, contentTypeToken, contentTypeValue))
)

// GetContentType takes a string representing the a Content-Type email
// header value and parses it into a set of values.
//
// According to http://www.w3.org/Protocols/rfc1341/4_Content-Type.html,
// the proper format for Content-Type is given by
//   Content-Type := type "/" subtype *[";" parameter]
//
//   type :=   "application"     / "audio"
//             / "image"           / "message"
//             / "multipart"  / "text"
//             / "video"           / x-token
//
//   x-token := <The two characters "X-" followed, with no
//              intervening white space, by any token>
//
//   subtype := token
//
//   parameter := attribute "=" value
//
//   attribute := token
//
//   value := token / quoted-string
//
//   token := 1*<any CHAR except SPACE, CTLs, or tspecials>
//
//   tspecials :=  "(" / ")" / "<" / ">" / "@"  ; Must be in
//              /  "," / ";" / ":" / "\" / <">  ; quoted-string,
//              /  "/" / "[" / "]" / "?" / "."  ; to use within
//              /  "="                        ; parameter values
func GetContentType(contentType string) (content, subtype string, param map[string]string, err error) {
	matches := contentTypeRegex.FindStringSubmatch(contentType)
	if len(matches) < 2 {
		return "", "", nil, errors.New("Cannot parse")
	}

	param = make(map[string]string)
	for i := 3; i+1 < len(matches); i += 2 {
		param[matches[i]] = matches[i+1]
	}

	return matches[1], matches[2], param, nil
}

// getPlainBody retrieves the plaintext body from the e-mail.
func getPlainBody(email *data.Content) (string, error) {
	contentType, ok := email.Headers["Content-Type"]
	if !ok {
		return "", errors.New("Unrecognized MIME version")
	}
	content, subtype, _, err := GetContentType(contentType[0])
	if err != nil {
		return "", err
	}

	if content == "text" && subtype == "plain" {
		return email.Body, nil
	}

	return "", nil
}

// GetSMTPBody return the body of an e-mail to be delivered through SMTP.
func GetSMTPBody(email *data.Content) (string, error) {
	if version, ok := email.Headers["MIME-Version"]; ok {
		if version[0] != "1.0" {
			return "", errors.New("Unrecognized MIME version")
		}

		// Case 1: message just has type text/plain
		contentType, ok := email.Headers["Content-Type"]
		if !ok {
			return "", errors.New("Unrecognized MIME version")
		}
		content, subtype, _, err := GetContentType(contentType[0])
		if err != nil {
			return "", err
		}
		if content == "text" && subtype == "plain" {
			return email.Body, nil
		}

		// Case 2: message has type mime/alternative, so get text/plain
		if content == "multipart" && subtype == "alternative" {
			for _, part := range email.ParseMIMEBody().Parts {
				body, _ := getPlainBody(part)
				if body != "" {
					return body, nil
				}
			}
			return "", errors.New("Couldn't find a text/plain MIME part")
		}

		// TODO we should be able to support html bodies eventually.
		return "", fmt.Errorf("Unsupported Content-Type: %s; use text/plain instead", contentType[0])
	}
	return email.Body, nil
}
