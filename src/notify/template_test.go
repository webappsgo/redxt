package notify

import "testing"

func TestParseTemplate(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantErr  bool
		wantSubj string
		wantBody string
	}{
		{
			name:     "valid",
			raw:      "Subject: Hello {app_name}\n---\nBody line one\nBody line two\n",
			wantSubj: "Hello {app_name}",
			wantBody: "Body line one\nBody line two\n",
		},
		{
			name:    "empty",
			raw:     "",
			wantErr: true,
		},
		{
			name:    "missing subject prefix",
			raw:     "Hello\n---\nbody\n",
			wantErr: true,
		},
		{
			name:    "missing separator",
			raw:     "Subject: Hello\nbody with no separator\n",
			wantErr: true,
		},
		{
			name:     "empty body",
			raw:      "Subject: Hello\n---\n",
			wantSubj: "Hello",
			wantBody: "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTemplate(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Subject != tt.wantSubj {
				t.Errorf("Subject = %q, want %q", got.Subject, tt.wantSubj)
			}
			if got.Body != tt.wantBody {
				t.Errorf("Body = %q, want %q", got.Body, tt.wantBody)
			}
		})
	}
}

func TestRender(t *testing.T) {
	tests := []struct {
		name string
		in   string
		vars map[string]string
		want string
	}{
		{
			name: "substitutes known variable",
			in:   "Hello {name}!",
			vars: map[string]string{"name": "World"},
			want: "Hello World!",
		},
		{
			name: "leaves unknown variable untouched",
			in:   "Hello {name}, code {missing}",
			vars: map[string]string{"name": "World"},
			want: "Hello World, code {missing}",
		},
		{
			name: "no variables",
			in:   "Plain text",
			vars: map[string]string{},
			want: "Plain text",
		},
		{
			name: "repeated variable",
			in:   "{x} and {x} again",
			vars: map[string]string{"x": "1"},
			want: "1 and 1 again",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Render(tt.in, tt.vars)
			if got != tt.want {
				t.Errorf("Render() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVariables(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "none", in: "plain text", want: nil},
		{name: "one", in: "hello {name}", want: []string{"name"}},
		{name: "dedupes in order", in: "{b} {a} {b}", want: []string{"b", "a"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Variables(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("Variables() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Variables()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		tmplName  string
		tmpl      Template
		wantErrs  int
		wantWarns bool
	}{
		{
			name:     "empty subject and body",
			tmplName: "test",
			tmpl:     Template{Subject: "", Body: ""},
			wantErrs: 2,
		},
		{
			name:     "unknown variable exact",
			tmplName: "test",
			tmpl:     Template{Subject: "s", Body: "{totally_bogus_name_xyz}"},
			wantErrs: 1,
		},
		{
			name:     "unknown variable with suggestion",
			tmplName: "test",
			tmpl:     Template{Subject: "s", Body: "{app_nam}"},
			wantErrs: 1,
		},
		{
			name:     "account template missing recipient_email",
			tmplName: "password_reset",
			tmpl:     Template{Subject: "s", Body: "{reset_link} {expires} {ip}"},
			wantErrs: 1,
		},
		{
			name:     "valid account template",
			tmplName: "password_reset",
			tmpl:     Template{Subject: "s", Body: "{recipient_email} {reset_link} {expires} {ip} did not request this"},
		},
		{
			name:     "valid non-account template",
			tmplName: "backup_complete",
			tmpl:     Template{Subject: "s", Body: "{filename} {size} {app_name}"},
		},
		{
			name:      "long subject warns",
			tmplName:  "test",
			tmpl:      Template{Subject: "This subject line is intentionally made extremely long so that it exceeds seventy eight characters total", Body: "body"},
			wantWarns: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Validate(tt.tmplName, tt.tmpl)
			if len(res.Errors) != tt.wantErrs {
				t.Errorf("Errors = %v (%d), want %d errors", res.Errors, len(res.Errors), tt.wantErrs)
			}
			if tt.wantWarns && len(res.Warnings) == 0 {
				t.Errorf("expected warnings, got none")
			}
			if tt.wantErrs == 0 && !res.OK() {
				t.Errorf("expected OK() true, errors: %v", res.Errors)
			}
		})
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"", "abc", 3},
		{"fqdn", "fqd", 1},
	}
	for _, tt := range tests {
		got := levenshtein(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
