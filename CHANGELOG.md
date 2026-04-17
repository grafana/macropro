# Changelog

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
