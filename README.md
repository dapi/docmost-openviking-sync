# Docmost → OpenViking Sync

Однонаправленная синхронизация страниц Docmost в Markdown-resources OpenViking.
Docmost не изменяется.

## Installation

Install the latest release for Linux or macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/dapi/docmost-openviking-sync/master/scripts/install.sh | sh
```

Set `VERSION=v0.1.0` to pin a release and `INSTALL_DIR=/usr/local/bin` for a
different destination. The installer verifies the downloaded archive SHA-256.

## Run

```sh
cp config.example.json config.json
docmost-openviking-sync -config config.json sync
docmost-openviking-sync -config config.json daemon
```

Файл конфигурации необязателен. Приоритет значений:

```text
CLI arguments > environment variables > JSON config > defaults
```

Поэтому контейнер можно запускать только с ENV:

```sh
DOCMOST_API_URL=https://docmost.example.com/api \
DOCMOST_API_TOKEN=... \
DOCMOST_WEBHOOK_SECRET=... \
docmost-openviking-sync daemon
```

Локальный OpenViking используется по умолчанию: `http://127.0.0.1:1933` с
приватным root `viking://user/resources/docmost`. Если сервер требует
авторизацию, передайте `OPENVIKING_API_KEY`.

| JSON | ENV | CLI argument | Default |
| --- | --- | --- | --- |
| `mode` | `SYNC_MODE` | `--mode` или `sync`/`daemon` | `sync` |
| — | `SYNC_CONFIG` | `--config` | без файла |
| `docmost.url` | `DOCMOST_API_URL` (`DOCMOST_URL`) | `--docmost-url` | — |
| `docmost.token` | `DOCMOST_API_TOKEN` (`DOCMOST_TOKEN`) | `--docmost-token` | — |
| `docmost.email` | `DOCMOST_EMAIL` | `--docmost-email` | — |
| `docmost.password` | `DOCMOST_PASSWORD` | `--docmost-password` | — |
| `openviking.url` | `OPENVIKING_URL` | `--openviking-url` | `http://127.0.0.1:1933` |
| `openviking.api_key` | `OPENVIKING_API_KEY` | `--openviking-api-key` | пусто |
| `openviking.root` | `OPENVIKING_ROOT` | `--openviking-root` | `viking://user/resources/docmost` |
| `state_path` | `SYNC_STATE_PATH` | `--state-path` | `data/state.json` |
| `interval` | `SYNC_INTERVAL` | `--interval` | `24h` |
| `allowlist` | `DOCMOST_SPACE_ALLOWLIST` | `--allowlist` | все spaces |
| `denylist` | `DOCMOST_SPACE_DENYLIST` | `--denylist` | пусто |
| `webhook.listen` | `WEBHOOK_LISTEN` | `--webhook-listen` | `:8080` |
| `webhook.path` | `WEBHOOK_PATH` | `--webhook-path` | `/events/docmost` |
| `webhook.secret` | `DOCMOST_WEBHOOK_SECRET` | `--webhook-secret` | пусто, receiver выключен |
| `webhook.debounce` | `WEBHOOK_DEBOUNCE` | `--webhook-debounce` | `10s` |

ENV и аргументы allowlist/denylist — списки через запятую. В Kubernetes
секреты следует передавать из `Secret` через ENV. Секретные CLI-аргументы
поддерживаются для полноты, но могут быть видны в списке процессов.

Идентичность ресурса: `<root>/pages/<page_id>.md`. По умолчанию root — приватный
`viking://user/resources/docmost`; публикация Docmost в общий
`viking://resources` требует осознанной настройки. Локальный state-файл
хранит только идентификаторы, URI и SHA-256 представлений страниц, поэтому
повторный запуск не вызывает лишнюю запись. Удаление применяется только к
space, чьё листинг-сканирование завершилось успешно.

`sync` печатает один JSON-отчёт в stdout и возвращает ненулевой код при любой
ошибке страницы. `daemon` запускает такую же попытку немедленно, принимает
HMAC-подписанные события Docmost и выполняет защитную полную сверку по
`interval` (по умолчанию раз в сутки).

Для событийного режима задайте одинаковый `OUTGOING_WEBHOOK_SECRET` в Docmost
и `DOCMOST_WEBHOOK_SECRET` здесь. Docmost должен отправлять события на
`http://<service>:8080/events/docmost`. Серия быстрых событий объединяется в
одну синхронизацию с помощью `webhook.debounce`.

Синхронизатор проверяет заголовок `X-Docmost-Signature-256` как HMAC-SHA256 от
точного тела запроса и отвечает `202 Accepted`. Периодическая сверка остаётся
обязательной страховкой от пропущенных или окончательно исчерпавших retries
webhook-доставок.

## Releases

Create and push a version tag, then start **Release** manually in GitHub
Actions with that tag. The workflow verifies the code, builds Linux and macOS
archives, publishes checksums, and creates the GitHub release.
