package ai

import "testing"

func TestStripCodeFence(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"```cpp\nint x;\n```", "int x;"},
		{"```code```", "code"},
		{"  ```code```  ", "code"},
		{"```\nno lang\n```", "no lang"},
		{"```js\nconst a = 1;", "const a = 1;"},
		{"plain code", "plain code"},
		{"  plain code  ", "plain code"},
		{"\n\t multiline\n plain \t", "multiline\n plain"},
	}
	for _, c := range cases {
		if got := stripCodeFence(c.in); got != c.want {
			t.Errorf("stripCodeFence(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
