package models

import "regexp"

// e164 matches an international phone number: "+", a non-zero country code
// digit, then 7-14 more digits (15 digits max in total).
var E164 = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
