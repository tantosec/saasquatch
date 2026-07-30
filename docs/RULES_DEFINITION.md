### Rule Definition

The core of `SaaSquatch` is its rule engine, which runs checks defined in simple YAML files. Each file starts with `version: 1` and a `rules:` list, so you can place multiple rule definitions in a single `.yml` file. Each rule is a self-contained test for a specific SaaS platform endpoint.

Each rule contains the `IDENTIFIER` placeholder which will be tested when the rule is run. The `known_good` is an identifier known to exist. The placeholder token is configurable via `identifier-placeholder` in the config file (default `IDENTIFIER`), so rules written with the old placeholder token still work if you set `identifier-placeholder` to the old value.

A rule's matching logic is a Sigma-style hybrid: a `detection` map of **named leaf matchers**, plus a `condition` expression string that combines them with `and` / `or` / `not` and parentheses. A leaf is a single `status`, `body`, or `header` check.

Below is a detailed breakdown of a rule's structure and all available fields.

#### Example Rule

```yaml
version: 1
rules:
  # When an org page exists, Github redirects to that org's home page —
  # unique behaviour we detect with a single named leaf and a trivial condition.
  - name: GitHub Organization
    service: GitHub
    tags:
      - dev
      - vcs
      - ci-cd
    known_good: google
    request:
      method: GET
      path: github.com/orgs/IDENTIFIER
      port: 443
      protocol: https
    detection:
      redirect: { header: { Location: ["https://github.com/IDENTIFIER"] } }
    condition: redirect
```

#### Composition Example

```yaml
    detection:
      logged_in:  { status: [200] }
      tenant_hdr: { header: { X-Tenant: ["IDENTIFIER"] } }
      not_found:  { body: ["No such organization"] }
    condition: (logged_in or tenant_hdr) and not not_found
```

---

### Field Reference

#### Top-Level Fields

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `name` | String | **Yes** | A unique, human-readable name for the rule. Used for logging and output. Example: `"GitHub Organization Check"`. |
| `service` | String | **Yes** | The name of the SaaS platform this rule targets. Used for grouping findings. Example: `"Salesforce"`. |
| `tags` | List of Strings | No | A list of tags for filtering and organising rules. See the [Tagging Guidelines](./RULE_TAGGING_GUIDE.md). |
| `known_good` | String | **Yes** | An identifier that is **known to be valid** for this service. This is used in "Test Mode" to verify that the rule's logic correctly identifies a valid presence. Example: `"google"`. |
| `request`| Object | **Yes** | An object containing the details of the HTTP request to be made. See `Request Object` section below. |
| `detection` | Map | **Yes** | A map of named leaf matchers. Each key is a name you reference in `condition`; each value is a single leaf. See `detection` section below. |
| `condition` | String | **Yes** | A boolean expression over the `detection` names using `and` / `or` / `not` and parentheses. The rule reports a find when this evaluates to true. Example: `(a or b) and not c`. |
| `captures` | Map | No | A map of named regexes that extract values from a matched response (e.g. an SSO link or tenant UUID). Only run when the rule fires. See `captures` section below. |

---

#### `request` Object

This object defines the HTTP request that will be sent to the target service.

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `method` | String | **Yes** | The HTTP method to use for the request. |
| `path` | String | **Yes** | The URL path to test. This string **must** contain the `IDENTIFIER` placeholder, which will be replaced with the identifier being tested. Example: `"IDENTIFIER.slack.com"`. |
| `protocol`| String | **Yes** | The protocol to use. Must be either `"http"` or `"https"` |
| `port` | Integer | No | The TCP port to connect to. If omitted, it defaults to `80` for `http` and `443` for `https`. |

---

#### `detection` and leaves

`detection` is a map of name → leaf. Each leaf sets **exactly one** of `status`, `body`, or `header`.

| Field | Type | Description |
| :--- | :--- | :--- |
| `status` | List of Integers | Matches if the response status code is in the list. Example: `[200, 301]`. |
| `body` | List of Strings | Regex patterns matched against the response body. Matches if **any** pattern matches. |
| `header` | Map of String → List of Strings | Per-header regex. Each key is a header name; its patterns are matched against that header's value(s). Matches if **every** named header matches at least one of its patterns. |
| `caseSensitive` | Boolean | Applies to `body`/`header` regex. **Defaults to `false`** (case-insensitive). Set to `true` for an exact case match. |

Within a leaf, a list of values is OR-linked (any match) and multiple header names are AND-linked (all must match). Anything beyond that — combining leaves, negation — is expressed in `condition`.

#### `condition`

A boolean expression over the leaf names: operators `and`, `or`, `not`, and parentheses for grouping. `IDENTIFIER` in any regex is replaced with the identifier under test. Examples:

- `match` — single leaf.
- `not identifier_not_found` — negation (replaces the old `positive: false`).
- `(a or b) and not (c or d)`.

#### `captures`

`captures` extracts useful values from a response **after** the rule fires — a login/SSO link, a tenant UUID, a redirect target. It is orthogonal to detection: captures never run on a negative, and the response body is only read for captures when a rule that fired actually defines them.

`captures` is a map of name → capture. Each capture's `regex` runs over the **whole serialized response** (headers as `Name: value` lines, then the body), so a single regex can target a `Location` header or body content alike. Note `.` does not match newlines by default — prefix `(?s)` to span lines.

| Field | Type | Description |
| :--- | :--- | :--- |
| `regex` | String | **Required.** Regex run against the response. The first capturing group is the extracted value, or the whole match if the regex has no group. `IDENTIFIER` is replaced with the identifier under test. |
| `all` | Boolean | **Defaults to `false`** (first match only). Set `true` to return every match as a list. |

```yaml
    captures:
      sso_link:   { regex: '(?s)Location: (https://\S+)' }
      tenant_uuid: { regex: 'data-tenant="([0-9a-f-]{36})"' }
      admins:     { regex: '([\w.+-]+@IDENTIFIER\.com)', all: true }
```

Captured values appear in JSON output under `captures` and are appended to the human `[+] FOUND` line.

---