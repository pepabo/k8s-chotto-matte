# k8s-chotto-matte

Podのreadinessを、監視対象の条件がパスするまで遅らせるためのツールです。

initContainerとして動かし、設定した条件（例: Unleashのフィーチャーフラグ）が指定回数連続で成功するまでプロセスを終了させないことで、本来のコンテナの起動を「ちょっと待って」もらいます。

## 使い方

TOML形式の設定ファイルを用意し、`--config` で渡してください（デフォルトは `/etc/k8s-chotto-matte/config.toml`）。

```toml
[[checks]]
name = "example"
type = "unleash"
interval_ms = 5000
success_threshold = 3
timeout_ms = 3000
fail_open = false

[checks.unleash]
url = "https://unleash.example.com/api"
flag_name = "example-flag"
expected_value = true
```

Unleashのトークンはconfigファイルには書かず、環境変数 `CHOTTOMATTE_UNLEASH_TOKEN_<チェック名(upper snake case)>` で渡してください。

すべての `checks` がそれぞれ `success_threshold` 回連続で成功すると、プロセスは正常終了し、Podのreadinessが通ります。

## サポートしているチェック種別

- `unleash`: Unleashのフィーチャーフラグの値を監視します

## 開発

```bash
make build   # ビルド
make test    # テスト
make lint    # lint
make format  # フォーマット
```

Go 1.26以降が必要です（`mise.toml` 参照）。
