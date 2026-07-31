# Changelog

## [1.1.0](https://github.com/grafana/macropro/compare/v1.0.2...v1.1.0) (2026-07-31)


### 🎉 Features

* **defaults:** dual-mode timeFrom/timeTo with quoted value form ([#31](https://github.com/grafana/macropro/issues/31)) ([4b96fe9](https://github.com/grafana/macropro/commit/4b96fe9dc71d5d521e8ee35f454fc4dbc1b8171e))
* **defaults:** gtime-compatible timeGroup intervals and dialect recipes ([#32](https://github.com/grafana/macropro/issues/32)) ([c1a977f](https://github.com/grafana/macropro/commit/c1a977fbeda20457e8bea2ee3c9a1d732b8c1400))
* **defaults:** match gtime.FormatInterval byte-for-byte in FormatDuration ([#33](https://github.com/grafana/macropro/issues/33)) ([a681e57](https://github.com/grafana/macropro/commit/a681e57a8ef9c5800edf03a7483f9905e8dc4348))


### 🔧 Chores

* **deps:** update actions/checkout action to v7.0.1 ([#28](https://github.com/grafana/macropro/issues/28)) ([83c41f2](https://github.com/grafana/macropro/commit/83c41f24a6947f6744cfb5300b05856e1411a954))
* use shared data-sources/base Renovate preset ([#29](https://github.com/grafana/macropro/issues/29)) ([1a33385](https://github.com/grafana/macropro/commit/1a33385adae52751608ca949fe090d6309d3d049))

## [1.0.2](https://github.com/grafana/macropro/compare/v1.0.1...v1.0.2) (2026-07-24)


### 🐛 Bug Fixes

* append trailing commenter tag once in Interpolate ([#22](https://github.com/grafana/macropro/issues/22)) ([cbe5702](https://github.com/grafana/macropro/commit/cbe5702d94e67fccd0c880d6f8220ec3c5be14a2))
* preserve trailing SQLCommenter tags across interpolation ([#17](https://github.com/grafana/macropro/issues/17)) ([635503d](https://github.com/grafana/macropro/commit/635503d93a8569fd12071c62fdab47e652275777))


### 🤖 Continuous Integration

* use shared reusable add-to-project workflow ([#27](https://github.com/grafana/macropro/issues/27)) ([1af8413](https://github.com/grafana/macropro/commit/1af84130a9afb9e899937c6ae2b8e0303ef3a89e))
* use shared reusable stale workflow ([#26](https://github.com/grafana/macropro/issues/26)) ([9d33a29](https://github.com/grafana/macropro/commit/9d33a29b129a6eb34085486681382c20104c2082))


### 🔧 Chores

* **deps:** update actions/setup-go action to v7 ([#25](https://github.com/grafana/macropro/issues/25)) ([2c12c92](https://github.com/grafana/macropro/commit/2c12c92ef6f4734ee7dae35459f9e9be0b0aba68))

## [1.0.1](https://github.com/grafana/macropro/compare/v1.0.0...v1.0.1) (2026-07-17)


### 🐛 Bug Fixes

* expand macros nested inside macro arguments ([#20](https://github.com/grafana/macropro/issues/20)) ([52d2492](https://github.com/grafana/macropro/commit/52d2492ec43f4b5a586ab22a500a3426268bccb8))


### 📝 Documentation

* add Apache 2.0 LICENSE ([e423b61](https://github.com/grafana/macropro/commit/e423b6100878227ad6bab6d179a885d5291aea80))
* add CI, pkg.go.dev, and Go Report Card badges to README ([5f55898](https://github.com/grafana/macropro/commit/5f558985f6a980d36d2926537e9f7913d0f39922))
* add CONTRIBUTING.md ([bbb74bb](https://github.com/grafana/macropro/commit/bbb74bb48d3a0438a8a1b6b1d8cfadd87e409f0b))
* add SECURITY.md ([ee27aa5](https://github.com/grafana/macropro/commit/ee27aa5331c9132fb40dea76f95793504974ab58))
* recommend sqlds Interpolator extension point in migration guide ([#18](https://github.com/grafana/macropro/issues/18)) ([75c285e](https://github.com/grafana/macropro/commit/75c285ea630c5200076f0313e12608e99442b585))


### 🤖 Continuous Integration

* add stale issue and PR workflow ([#19](https://github.com/grafana/macropro/issues/19)) ([cfa834d](https://github.com/grafana/macropro/commit/cfa834dd164e35e68914ea6e7f8b50dc8b1643ed))
* drop component prefix from release-please tags ([5d21dd7](https://github.com/grafana/macropro/commit/5d21dd7177710e80d091464687ab9d88d4f2c700))
* harden GitHub Actions workflows ([#12](https://github.com/grafana/macropro/issues/12)) ([dbfa76a](https://github.com/grafana/macropro/commit/dbfa76ad70954bd9b8e479ac15f180d86f93bf77))


### 🔧 Chores

* add `add-to-project workflow` ([#14](https://github.com/grafana/macropro/issues/14)) ([abbd352](https://github.com/grafana/macropro/commit/abbd35242ce35491575ffce971a6a0a77adc83f1))
* add CODEOWNERS ([a411967](https://github.com/grafana/macropro/commit/a4119676938c187d7117c6a98f5e1388be3149f5))
* **deps:** update actions/checkout action to v7 ([#13](https://github.com/grafana/macropro/issues/13)) ([681538a](https://github.com/grafana/macropro/commit/681538a9afb1ee6a287cf68e362f627eaba58d42))
* **deps:** update actions/setup-go action to v6.5.0 ([#15](https://github.com/grafana/macropro/issues/15)) ([6f3fa7c](https://github.com/grafana/macropro/commit/6f3fa7ca36bd5003b342132dd17030a3979f2041))
* **deps:** update dependency golangci/golangci-lint to v2.12.2 ([#9](https://github.com/grafana/macropro/issues/9)) ([8ac3db5](https://github.com/grafana/macropro/commit/8ac3db5aa0a8c09528ac9add2dc78e8015301bb5))
* **deps:** update golangci/golangci-lint-action action to v9.3.0 ([#16](https://github.com/grafana/macropro/issues/16)) ([8e360bf](https://github.com/grafana/macropro/commit/8e360bfd5e993859ee8c11301a8502b0291f01f4))
* **deps:** update googleapis/release-please-action action to v5 ([#10](https://github.com/grafana/macropro/issues/10)) ([47972f1](https://github.com/grafana/macropro/commit/47972f19595c0fe2b2f4fb98c0724331ec061cb8))

## 1.0.0 (2026-04-17)


### 🎉 Features

* add BacktickQuote, BracketQuote, BackslashEscape comment-style flags ([44a480e](https://github.com/grafana/macropro/commit/44a480e23e544d0f540a2cec7d7922c1d12fcb30))
* add WithPrefix and WithComments options for non-SQL dialects ([21963a8](https://github.com/grafana/macropro/commit/21963a80d2d175d2105e510fae61a02f98d05734))


### 🐛 Bug Fixes

* recover from panics inside macro handlers ([29b4004](https://github.com/grafana/macropro/commit/29b4004b7ef9a5e1f8b9a9c759f5ca4b885ef849))
* return caller's original query on Interpolate error, not the stripped copy ([d5877c9](https://github.com/grafana/macropro/commit/d5877c9a3598ea33b69e4a0d816ece4e8964be2a))
* strip SQL comments before interpolating macros ([a26cd4b](https://github.com/grafana/macropro/commit/a26cd4b7e4c0202a4009751fd1cb8e47e85a5afd))
* strip unterminated block comments through EOF and handle \r terminators ([092bbe5](https://github.com/grafana/macropro/commit/092bbe569ff0397465f5df6ae39f6fd6115319fc))


### 📝 Documentation

* add security considerations section to README ([b956cb5](https://github.com/grafana/macropro/commit/b956cb5f9661fb67217dd6101c9072560fe8e1d2))


### ♻️ Code Refactoring

* rename needle constants to describe comment styles ([61b4802](https://github.com/grafana/macropro/commit/61b4802a2d762b97122d5d1d574a0f37f7fed328))
* unify string-literal scanning into scanStringLiteral ([7b35407](https://github.com/grafana/macropro/commit/7b354074e68bcf5de933698edde24ba82e4d18e2))


### ⚡ Performance Improvements

* batch writes in StripComments scanner via strings.IndexAny ([1cc5b10](https://github.com/grafana/macropro/commit/1cc5b1051a542cc3a14566f7f2e570150d0224ff))
* expand macros in a single forward pass ([7eac86c](https://github.com/grafana/macropro/commit/7eac86c4d6f5b882fa6f1c23ed5228775eb37f8e))
* fast-path StripComments when no comment starters are present ([aaa0df1](https://github.com/grafana/macropro/commit/aaa0df18bc6e2767d15cab7edae23f79ecaf1bed))
* short-circuit Interpolate when no prefix is present ([c29699e](https://github.com/grafana/macropro/commit/c29699eb84e4d60b28c2758723c94760de84b6ef))


### ✅ Tests

* add benchmark suite for future performance work ([18282bf](https://github.com/grafana/macropro/commit/18282bf2d229779de2ea31a08abaa169ab8a6e62))


### 🤖 Continuous Integration

* add GitHub Actions workflow with test, lint, and govulncheck jobs ([85342e0](https://github.com/grafana/macropro/commit/85342e054c141a849eb4d172d37549ae31d28e4b))
* enforce Conventional Commits format on PR titles ([6f8b3b6](https://github.com/grafana/macropro/commit/6f8b3b67c54bac8676030c78e3607ac23477d16b))
* fix gofmt, revive, and go.sum-diff failures ([8f03201](https://github.com/grafana/macropro/commit/8f032010ad98a2eafdd2ae0bc5faf23f0c01a33f))


### 🔧 Chores

* support Go 1.23+ and test across 1.23.x/1.25.x ([0bae194](https://github.com/grafana/macropro/commit/0bae194cd3d53e2a12582729db62dcc36f506d9f))
