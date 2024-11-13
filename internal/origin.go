package internal

import "net/url"

func Origin(u *url.URL) string {
	if u == nil {
		return ""
	}

	base := u.Scheme + "://" + u.Hostname()

	if port := u.Port(); len(port) > 0 {
		if u.Scheme == "http" && u.Port() != "80" {
			base += ":" + u.Port()
		} else if u.Scheme == "https" && u.Port() != "443" {
			base += ":" + u.Port()
		}
	}

	return base
}

func ValidateCanonicalOrigin(v string) bool {
	ou, err := url.Parse(v)
	if err != nil {
		return false
	}

	return Origin(ou) == v
}
