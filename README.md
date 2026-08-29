# Docmost → OpenViking Sync

Однонаправленная синхронизация страниц Docmost в Markdown-resources OpenViking.
Docmost не изменяется.

## Installation

Install the latest release for Linux or macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/dapi/docmost-openviking-sync/main/scripts/install.sh | sh
```

Set `VERSION=v0.1.0` to pin a release and `INSTALL_DIR=/usr/local/bin` for a
different destination. The installer verifies the downloaded archive SHA-256.

## Run

```sh
cp config.example.json config.json
docmost-openviking-sync -config config.json sync
docmost-openviking-sync -config config.json daemon
```

Секреты не нужно хранить в `config.json`: параметры `DOCMOST_API_URL`,
`DOCMOST_API_TOKEN` (либо `DOCMOST_EMAIL` и `DOCMOST_PASSWORD`),
`OPENVIKING_URL` и `OPENVIKING_API_KEY` имеют приоритет над пустыми полями
файла. Для развёртывания загружайте их из `pass` через `.envrc`.

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
