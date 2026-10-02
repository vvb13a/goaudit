package checks

import (
	"testing"

	"github.com/vvb13a/goaudit/domain"
)

func TestBrokenTargetSeverity(t *testing.T) {
	cases := []struct {
		code   int
		err    string
		sev    domain.Severity
		broken bool
	}{
		{200, "", domain.SeveritySuccess, false},
		{301, "", domain.SeveritySuccess, false},
		{403, "", domain.SeverityWarning, true},
		{429, "", domain.SeverityWarning, true},
		{404, "", domain.SeverityError, true},
		{410, "", domain.SeverityError, true},
		{500, "", domain.SeverityError, true},
		{503, "", domain.SeverityError, true},
		{0, "connection refused", domain.SeverityError, true},
	}
	for _, c := range cases {
		sev, broken := brokenTarget(domain.TargetStatus{StatusCode: c.code, Error: c.err})
		if broken != c.broken || sev != c.sev {
			t.Errorf("code=%d err=%q: got (%s,%v), want (%s,%v)", c.code, c.err, sev, broken, c.sev, c.broken)
		}
	}
}
