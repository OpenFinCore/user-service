package utils

import (
	"fmt"
	"math"
	"net"
	"strings"
	"time"
)

// normalizePhone strips the formatting characters people commonly type
// ("+92 300-123 (4567)") and converts a leading "00" to "+".
func NormalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", ".", "").Replace(phone)
	if strings.HasPrefix(phone, "00") {
		phone = "+" + phone[2:]
	}
	return phone
}

// maskPhone keeps the country prefix and last 4 digits: "+92******4567".
func MaskPhone(phone string) string {
	if len(phone) <= 7 {
		return "****"
	}
	return phone[:3] + strings.Repeat("*", len(phone)-7) + phone[len(phone)-4:]
}

func OtpSMSText(code string, validFor time.Duration) string {
	minutes := int(math.Ceil(validFor.Minutes()))
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("Your verification code is %s. It expires in %d minutes. Do not share it with anyone.", code, minutes)
}

func ParseIP(s string) *net.IP {
	ip := net.ParseIP(s)
	if ip == nil {
		return nil
	}
	return &ip
}

func OptionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func Truncate(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}
