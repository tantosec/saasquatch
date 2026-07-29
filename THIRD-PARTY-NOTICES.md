# Third-Party Notices

SaaSquatch is distributed under the MIT License (see [LICENSE](./LICENSE)). It
statically links Go dependencies that carry their own licences. Those licences
apply to the corresponding portions of any distributed binary.

## Mozilla Public License 2.0

* **github.com/hashicorp/golang-lru/v2** — MPL-2.0
  Source: <https://github.com/hashicorp/golang-lru>
  Licence: <https://github.com/hashicorp/golang-lru/blob/main/LICENSE>

This dependency is used unmodified. Under MPL-2.0 §3.2, the source code for the
MPL-covered files is available under the MPL at the URL above.

## Permissive licences

All remaining dependencies are licensed under MIT, BSD (2- and 3-clause), or
Apache-2.0. The authoritative licence for each is the `LICENSE` file in the
corresponding module, resolvable from `go.mod` / `go.sum`. To reproduce the full
list locally:

```sh
go list -m all
```
