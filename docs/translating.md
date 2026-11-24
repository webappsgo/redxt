# Translating redxt

redxt translates every human-readable string shown by the server, admin panel, API, CLI, agent, email, and legal pages. English (`en`) is the source locale. Translators update values in the locale files; message keys and their meaning stay stable.

## Current locale files

The repository currently embeds seven locale files:

| Code | Language | Direction |
|---|---|---|
| `en` | English | Left to right |
| `es` | Spanish | Left to right |
| `zh` | Chinese | Left to right |
| `fr` | French | Left to right |
| `ar` | Arabic | Right to left |
| `de` | German | Left to right |
| `ja` | Japanese | Left to right |

These are the locales currently registered in `src/common/i18n/i18n.go`. AI.md PART 31 also requires Italian (`it`) at minimum, and requires locale metadata plus a single registry for native names, direction, and plural categories. The current code and locale files do not yet provide those requirements: `it.json` is absent, the JSON catalogs are flat dot-key maps without a `meta` block, and language metadata/plural behavior is split across code declarations. Until that is corrected, do not add a new supported language or claim full PART 31 compliance. Existing locale updates must keep the current flat key structure and match `en.json` exactly.

## Editing a translation

For an existing language, edit only its locale file at `src/common/i18n/locales/{lang}.json`. Use `en.json` as the source for keys and ordering. Keep the file UTF-8 JSON with two-space indentation, no comments, and no trailing commas. Change values only; never translate, rename, remove, or reorder keys.

Translate complete phrases naturally. Do not assemble a sentence by joining fragments: word order, gender, and case differ across languages. Keep a consistent, respectful register and a concise, neutral tone. If an English value is ambiguous or is only a sentence fragment, report it for correction instead of working around it in another locale.

### Placeholders

Every placeholder must remain exactly intact, including its name and braces. For example, `{count}` must remain `{count}`. Translators may move a placeholder to the grammatically correct position, but may not rename, remove, or add one. Preserve literal user-facing values such as `dark` and `auto` when they are configuration values.

### Plurals

Preserve each plural key and provide the categories required by the language. The current runtime uses:

| Language | Categories |
|---|---|
| English, Spanish, German | `one`, `other` |
| French | `one`, `other` (`0` uses `one`) |
| Arabic | `zero`, `one`, `two`, `few`, `many`, `other` |
| Chinese, Japanese | `other` |

Follow the language's CLDR rules. Do not copy English singular/plural rules into another language just because the English catalog has two forms. All locale key sets must remain equal to English, including plural keys.

### Terms that stay untranslated

Keep product and brand names, protocol and technology names, code, filenames, configuration keys, URLs, DNS record-type mnemonics, and literal values users must enter unchanged. Examples include `redxt`, `API`, `DNS`, `DNSSEC`, `DoH`, `DoQ`, `DoT`, `I2P`, `Tor`, `.onion`, `JSON`, `TSIG`, `TXT`, `A`, `AAAA`, `MX`, `SRV`, `CAA`, `TLSA`, `dark`, and `auto`.

Use these product terms consistently:

| Term | Meaning to preserve |
|---|---|
| Authoritative DNS | Answers for zones hosted by this redxt instance. |
| Recursive resolver | Resolves names by following referrals and querying upstream DNS servers. |
| Zone | An administratively managed part of the DNS namespace and its records. |
| Record | A DNS resource record within a zone. |
| DDNS | Dynamic DNS updates to a host's DNS records. |
| TSIG | A shared-secret signature used to authenticate DNS messages. |
| DNSSEC | DNS data origin authentication and integrity validation. |
| Cluster node | Another redxt server instance participating in the same cluster. |
| Managed node | An external DNS or redirect target monitored by redxt. |
| Server Admin | A person who manages the redxt application. |
| Regular User | An end user account, separate from a Server Admin. |

Use one established target-language equivalent for each recurring term in a locale. Do not translate DNS record mnemonics, protocol names, or literal configuration values.

### Typography and accessibility

Follow the language's punctuation, quotation, decimal, and thousands-separator conventions. French text uses non-breaking spaces where required before `: ; ! ?`. Preserve meaningful capitalization, accents, and script direction.

Every translation must retain the source's accessible meaning. Keep action labels distinct, preserve form-label and error relationships, and do not remove text that conveys a warning or state. Arabic is right-to-left; verify that page direction and layout are mirrored correctly.

## Review before submitting

Open the changed screens at 320 px, 768 px, and 1280 px widths. Confirm that translated text is not clipped, overlaps controls, overflows its container, or breaks navigation. Check longer strings (which can be about 40% longer than English), keyboard focus, screen-reader labels, and right-to-left layout where applicable.

Before a translation is merged, maintainers check that:

- The locale has the same keys and plural categories as English.
- Placeholders are unchanged and all values are complete phrases.
- Terms follow the glossary and untranslated names remain intact.
- Text fits the supported widths and preserves accessibility.
- Locale documentation and credits are current.

Native-speaker review is welcome but is not required to merge. Human corrections take precedence over machine-generated wording. When English changes, update every locale in the same change where a reliable translation is available; do not leave stale keys or use a marker to hide untranslated text.

## Credits

No translator credits are currently recorded in the locale files. Translators may request a name or handle be credited here and in the locale metadata when the metadata format is implemented. Do not add a person's name without their consent.
