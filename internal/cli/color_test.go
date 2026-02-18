package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// hasANSI checks whether a string contains ANSI escape sequences.
func hasANSI(s string) bool {
	return strings.Contains(s, "\033[") || strings.Contains(s, "\x1b[")
}

func TestStyledWithColor(t *testing.T) {
	result := styled("hello", "\033[1m", true)
	testutil.Contains(t, result, "hello")
	testutil.True(t, hasANSI(result), "expected ANSI codes in styled output")
}

func TestStyledWithoutColor(t *testing.T) {
	result := styled("hello", "\033[1m", false)
	testutil.Equal(t, "hello", result)
}

func TestBold(t *testing.T) {
	r := bold("test", true)
	testutil.Contains(t, r, "test")
	testutil.True(t, hasANSI(r), "expected ANSI in bold output")
	testutil.Equal(t, "test", bold("test", false))
}

func TestDim(t *testing.T) {
	r := dim("test", true)
	testutil.Contains(t, r, "test")
	testutil.True(t, hasANSI(r), "expected ANSI in dim output")
	testutil.Equal(t, "test", dim("test", false))
}

func TestCyan(t *testing.T) {
	r := cyan("test", true)
	testutil.Contains(t, r, "test")
	testutil.True(t, hasANSI(r), "expected ANSI in cyan output")
	testutil.Equal(t, "test", cyan("test", false))
}

func TestGreen(t *testing.T) {
	r := green("test", true)
	testutil.Contains(t, r, "test")
	testutil.True(t, hasANSI(r), "expected ANSI in green output")
	testutil.Equal(t, "test", green("test", false))
}

func TestBoldCyan(t *testing.T) {
	r := boldCyan("test", true)
	testutil.Contains(t, r, "test")
	testutil.True(t, hasANSI(r), "expected ANSI in boldCyan output")
	testutil.Equal(t, "test", boldCyan("test", false))
}

func TestBoldGreen(t *testing.T) {
	r := boldGreen("test", true)
	testutil.Contains(t, r, "test")
	testutil.True(t, hasANSI(r), "expected ANSI in boldGreen output")
	testutil.Equal(t, "test", boldGreen("test", false))
}

func TestStyledEmptyString(t *testing.T) {
	r := styled("", "\033[1m", true)
	testutil.True(t, hasANSI(r), "expected ANSI even for empty string")
	testutil.Equal(t, "", styled("", "\033[1m", false))
}

func TestColorEnabledRespectsNO_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	testutil.False(t, colorEnabled())
}

func TestColorEnabledFdRespectsNO_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	// NO_COLOR is set (even to empty string), so color should be disabled.
	testutil.False(t, colorEnabledFd(os.Stderr.Fd()))
}

func TestColorEnabledNO_COLORNotSet(t *testing.T) {
	// When NO_COLOR is not set, colorEnabled depends on terminal detection.
	// In tests, stderr is not a terminal, so both return false.
	// Use t.Setenv first to snapshot+restore, then unset for this test.
	t.Setenv("NO_COLOR", "placeholder")
	os.Unsetenv("NO_COLOR")
	testutil.False(t, colorEnabled())
	testutil.False(t, colorEnabledFd(os.Stderr.Fd()))
}

func TestYellow(t *testing.T) {
	r := yellow("test", true)
	testutil.Contains(t, r, "test")
	testutil.True(t, hasANSI(r), "expected ANSI in yellow output")
	testutil.Equal(t, "test", yellow("test", false))
}
