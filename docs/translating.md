# Translating redxt

redxt translates human-facing strings used by the server, CLI, agent, admin panel, API, emails, and legal pages. English (`en`) is the source locale. Translators contribute changes to one locale at a time; keep keys and their meaning stable.

## Supported locales

The repository embeds eight complete locales. This count must match the JSON files in `src/common/i18n/locales/` and the language registry in `src/common/i18n/i18n.go`.

| Code | Language | Native name | Direction | Required plural categories |
|---|---|---|---|---|
| `en` | English | English | LTR | `one`, `other` |
| `es` | Spanish | Español | LTR | `one`, `other` |
| `zh` | Chinese | 中文 | LTR | `other` |
| `fr` | French | Français | LTR | `one`, `other` |
| `ar` | Arabic | العربية | RTL | `zero`, `one`, `two`, `few`, `many`, `other` |
| `de` | German | Deutsch | LTR | `one`, `other` |
| `it` | Italian | Italiano | LTR | `one`, `many`, `other` |
| `ja` | Japanese | 日本語 | LTR | `other` |

The required minimum is English, Spanish, French, Arabic, German, and Italian. Chinese and Japanese are additional supported locales. Language metadata and plural categories come from the single registry; each locale file's metadata must agree with it.
To add a language, contribute its complete locale file and registry entry together, then update the project's default available-language list and this table. The registry, embedded files, runtime defaults, and documented locale count must stay aligned.

## Editing a locale

Translate values in `src/common/i18n/locales/{lang}.json`. Do not edit `en.json` as part of a translation contribution. Keep UTF-8 JSON, two-space indentation, and no comments or trailing commas. Retain the nested message structure, key names, and key order used by English. A normal message is a string under its namespace; plural messages are grouped under `plurals` by message key:

```json
{
  "meta": {
    "language": "es",
    "name": "Spanish",
    "native_name": "Español",
    "direction": "ltr",
    "version": "1.0.0"
  },
  "common": {
    "save": "Guardar"
  },
  "plurals": {
    "items_count": {
      "one": "{count} elemento",
      "other": "{count} elementos"
    }
  }
}
```

Preserve the `meta` fields: `language`, English `name`, `native_name`, `direction`, and `version`. Translators may add their name or handle in `meta.translators` and request a credit in this guide. Never add a person's credit without consent.

Translate complete phrases naturally. Do not assemble sentences by joining fragments; word order, gender, and case differ across languages. Use one consistent, respectful register and a concise, neutral tone. If the English source is ambiguous or incomplete, report it for correction instead of working around it in another locale.

### Placeholders

Keep every named placeholder exactly as written, including its braces, such as `{count}`, `{name}`, or `{datetime}`. You may move it to the grammatically correct position, but do not rename or invent placeholders. In plural messages, `{count}` may be omitted in a category whose wording does not need it; all other placeholders must remain in every category.

### Plurals

Each locale must include one plural message for every English plural message. Include all required CLDR categories listed above. An explicit `zero` form is also allowed for any language, such as “No items”; other unused categories are not allowed. The runtime uses an explicit zero form when present, otherwise it selects the language's CLDR category and falls back to `other`.

### Terms that stay untranslated

Keep product and brand names, protocol and technology names, code, filenames, configuration keys, URLs, DNS record mnemonics, and literal values users must enter unchanged. Examples include `redxt`, `API`, `DNS`, `DNSSEC`, `DoH`, `DoQ`, `DoT`, `I2P`, `Tor`, `.onion`, `JSON`, `TSIG`, `TXT`, `A`, `AAAA`, `MX`, `SRV`, `CAA`, `TLSA`, `dark`, and `auto`. Preserve license-mandated attribution text and the English first-use offline fallback defined by the service worker.

Use these product terms consistently and preserve their meaning:

| Term | Meaning |
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
| Regular User | An end-user account, separate from a Server Admin. |
| Token | A credential used to authenticate an API or CLI request. |
| Rotate | Replace a credential or key and retire the previous value. |
| Revoke | Invalidate a credential or permission before its normal expiry. |
| Paste | Submit or insert text into an input field. |

Use established target-language equivalents consistently for glossary terms. Do not translate record mnemonics, protocol names, or literal configuration values.

### Typography and accessibility

Follow each language's punctuation, quotation, decimal, and thousands-separator conventions. French uses non-breaking spaces where required before `: ; ! ?`. Preserve accents, meaningful capitalization, and script direction. Keep action labels distinct, preserve form-label and error relationships, and retain warning and status meaning. Arabic is RTL; check that direction and layout mirror correctly.

## Review before submitting

Review changed screens at 320 px, 768 px, and 1280 px widths. Confirm text is not clipped, overlaps controls, overflows its container, or breaks navigation. Allow for strings up to about 40% longer than English. Check keyboard focus, screen-reader labels, and RTL layout where applicable.

Maintainers check message-key parity with English, required plural categories, placeholder consistency, glossary use, accessibility, and layout. Native-speaker review is welcome but not a merge gate. AI and machine-assisted translation is encouraged when the wording is reliable; human corrections take precedence. Keep every locale current with English changes when a reliable translation is available.

## Credits

Translator credits are recorded here and, with the translator's consent, in the locale file's `meta.translators` field. No translator credits are currently recorded.
