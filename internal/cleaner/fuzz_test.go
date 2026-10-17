package cleaner

import (
	"strings"
	"testing"
)

// FuzzClean checks, for any input (spec §7, cleaner):
//   - Clean never panics and always returns (the fuzzer reports hangs);
//   - the output is either the trimmed input, unchanged (non-http(s)), or
//     a valid http(s) URL;
//   - cleaning is idempotent: cleaning the output again changes nothing,
//     unless the redirect hop limit was reached.
func FuzzClean(f *testing.F) {
	for _, u := range benchURLs {
		f.Add(u)
	}
	for _, u := range []string{
		"", " ", "mailto:a@b.com", "example.com:8080/p?fbclid=1", "https://e.com/#:~:text=a=b&utm_source=x",
		"https://e.com/?&&utm_source=1&", "https://www.amazon.com/ref=x/s?k=1&ref=2",
		"https://www.amazon.com/dp/X/ref=a#b?c=d", "https://www.bigfishgames.com/x?pc",
		"https://pantip.com/topic/1#lead=1&utm_source=2",
		"https://www.google.com/url?q=https%253A%252F%252Fe.com%252F",
		"//example.com/?fbclid=1",
	} {
		f.Add(u)
	}

	c := bundled(f)
	f.Fuzz(func(t *testing.T, input string) {
		res := c.Clean(input)
		if res.Err != nil {
			if res.Output != "" || res.Changed {
				t.Fatalf("Clean(%q): error with output %q", input, res.Output)
			}
			return
		}

		if _, ok := httpHost(res.Output); !ok && res.Output != strings.TrimSpace(input) {
			t.Fatalf("Clean(%q) = %q: neither a valid http(s) URL nor the input unchanged", input, res.Output)
		}

		if res.HopLimitReached {
			return
		}
		again := c.Clean(res.Output)
		if again.Err != nil || again.Output != res.Output {
			t.Fatalf("not idempotent:\n input  %q\n clean  %q\n again  %q (err %v)\n removed %+v\n unwrapped %q",
				input, res.Output, again.Output, again.Err, again.Removed, again.Unwrapped)
		}
	})
}
