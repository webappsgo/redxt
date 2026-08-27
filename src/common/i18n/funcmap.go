package i18n

import "html/template"

// FuncMap returns the `t`, `tf`, and `tp` template functions bound to
// lang, for wiring into html/template.Funcs per PART 31's template
// integration requirement.
//
//	{{t "auth.login"}}
//	{{tf "welcome.message" "name" .Viewer.DisplayName}}
//	{{tp "items_count" .Count}}
func FuncMap(lang string) template.FuncMap {
	return template.FuncMap{
		"t": func(key string) string {
			return Translate(lang, key)
		},
		"tf": func(key string, pairs ...string) string {
			vars := make(map[string]string, len(pairs)/2)
			for i := 0; i+1 < len(pairs); i += 2 {
				vars[pairs[i]] = pairs[i+1]
			}
			return TranslateFormat(lang, key, vars)
		},
		"tp": func(key string, n int) string {
			return TranslatePlural(lang, key, n, nil)
		},
	}
}
