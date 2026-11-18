# Vercel: API и PostgreSQL

Проект: `ipodymovs-projects/blog-api`, ID `prj_AeHOZb8Zrk49OmBcLcMG53AZcwpY`.
Назначенный домен: `https://blog-api-mu-one.vercel.app`.
Наличие домена само по себе не означает успешный деплой.

## Архитектура

- Vercel Go runtime (Beta) запускает `cmd/api/main.go` и масштабирует API.
- PostgreSQL Neon хранит пользователей, статьи, токены и статистику.
  Для API нужен pooled `DATABASE_URL`, для миграций — `DATABASE_URL_UNPOOLED`.
  База и API размещаются во Франкфурте (`fra1`), подключение к БД использует TLS.
- `scripts/vercel-build.sh` применяет миграции с транзакционным advisory lock
  и проверкой checksum, затем собирает бинарник. Ошибка миграций останавливает сборку.
- Cron раз в день (`0 3 * * *`, UTC) вызывает `/internal/maintenance`.
  Отдельный `CRON_SECRET` защищает очистку от обычных пользователей.
- `/health`, `/ready`, `/docs`, `/openapi.yaml` обслуживаются самим Go API.
  Контракт встроен в бинарник и не зависит от файловой системы хостинга.

Отдельные Redis, очередь и файловое хранилище этому API сейчас не нужны.
Все постоянные данные хранятся в PostgreSQL, аватары являются внешними URL.

## Production-переменные

| Имя                     | Назначение                                                   |
| ----------------------- | ------------------------------------------------------------ |
| `DATABASE_URL`          | Pooled PostgreSQL URL от интеграции Neon                     |
| `DATABASE_URL_UNPOOLED` | Прямой URL от Neon для миграций                              |
| `VISITOR_SECRET`        | Стабильный случайный секрет гостевых cookie                  |
| `CRON_SECRET`           | Отдельный случайный секрет Vercel Cron                       |
| `COOKIE_SECURE`         | `true`                                                       |
| `PUBLIC_SITE_URL`       | Адрес для сайтов с пустым `public_url`; пока сайта нет — API |
| `ACCESS_TTL`            | `15m`                                                        |
| `REFRESH_TTL`           | `720h`                                                       |
| `VIEW_INTERVAL`         | `30m`                                                        |
| `MAX_STATISTICS_DAYS`   | `366`                                                        |

`PORT` задаёт Vercel; не переопределяйте `HTTP_ADDR` в хостинге.
Секреты не передаются в Git и не выводятся в логи.
Preview/Development должны использовать отдельную базу и отдельные секреты.
Production-база подключается только к Production. До настройки отдельной базы
preview-сборки намеренно не проходят миграции.

## Деплой и обновления

Из корня репозитория:

```sh
npx vercel@63.1.2 link --project blog-api --scope ipodymovs-projects --yes
make check
make build
npx vercel@63.1.2 deploy --prod --scope ipodymovs-projects
```

После статуса `READY` проверьте `/ready`, Swagger и операции API.

Проект подключён к `IPodymov/blog-api`, production-ветка `main`.
Для Git-автодеплоя нужно опубликовать текущие изменения в репозитории.
Каждая production-сборка применяет
миграции: используйте совместимые добавочные изменения схемы. Откат Vercel
не откатывает PostgreSQL.

Глобального администратора создайте командой `cmd/admin` с production-БД, а
демо-наполнение и smoke-проверку запускайте через `cmd/seed`
([docs/admin.md](admin.md)).

При первом деплое с ролями сразу назначьте `author` нужным аккаунтам, иначе они
не смогут создавать и редактировать статьи ([docs/sites.md](sites.md#обновление-до-ролей)).

## CORS и ключ сайта

CORS обрабатывает сам Go API, поэтому в `vercel.json` и в переменных окружения
он не настраивается. Переменная `CORS_ORIGINS` не нужна.
API принимает запросы с любого `Origin`. Сайт выбирается ключом из заголовка
`X-Site-Key`, заголовка `X-API-Key` или query-параметра `api_key`.

Проверка после деплоя:

```sh
curl -i 'https://blog-api-mu-one.vercel.app/api/v1/articles?api_key=default&limit=1' \
  -H 'Origin: https://example.com'
```

Ожидается `200` и `access-control-allow-origin: https://example.com`.
Пример подключения сайта — в [docs/sites.md](sites.md#подключение-сайта-одним-ключом).

## CI/CD

- CI: `.github/workflows/ci.yml` на каждый push в `main` и pull request запускает
  gofmt, `go mod tidy -diff`, `go vet` (включая тег `integration`), unit-тесты с race,
  интеграционные тесты на PostgreSQL 18, проверку OpenAPI (Redocly + сверка маршрутов)
  и сборку Docker-образа.
- CD: Vercel Git-интеграция деплоит `main` в Production, остальные ветки — в Preview.
  Сборка Vercel выполняется с `-buildvcs=false`: `.git` не попадает в окружение сборки.

## Эксплуатация

Логи доступны в Vercel Observability. `/ready` проверяет доступность БД.
Следите за статусом Cron и расходами Vercel/Neon. Возможности и срок хранения
резервных копий зависят от тарифа Neon; проверьте их в панели базы и периодически
проверяйте восстановление в отдельной ветке/БД.

Go-пул ограничен пятью соединениями на экземпляр, idle connections закрываются
через минуту. Встроенный rate limit действует на экземпляр, а не глобально
на весь autoscaling fleet. При росте нагрузки используйте правила Vercel Firewall
или общий rate limiter. Клиентский IP берётся из заголовка платформы только
при `VERCEL=1`; локальный сервер игнорирует такие заголовки.

Для гостевых cookie размещайте сайт и API на одном site или за same-origin proxy:
cookie имеет `SameSite=Lax`. При добавлении сайта создайте запись в `sites`
([docs/sites.md](sites.md)); новый деплой не нужен. API не реализует HTML-страницы статей.

Официальная документация: [Go runtime](https://vercel.com/docs/functions/runtimes/go),
[Cron](https://vercel.com/docs/cron-jobs/manage-cron-jobs),
[заголовки платформы](https://vercel.com/docs/headers/request-headers).

## Вход через отдельный HTTPS-прокси

[Конфигурация Caddy](../deploy/proxy/Caddyfile) рассчитана на отдельный сервер,
доступный и клиентам из РФ, и домену API на Vercel. В DNS направьте домен прокси
на этот сервер, откройте TCP 80 и 443 и задайте переменную окружения
`API_PROXY_DOMAIN` для процесса Caddy (имя домена без схемы). На сервере
с Caddy 2 из корня репозитория выполните:

```sh
caddy validate --config deploy/proxy/Caddyfile
caddy run --config deploy/proxy/Caddyfile
```

Caddy выпускает сертификат для домена и передаёт метод, путь, query и тело
запроса на фиксированный HTTPS-адрес Vercel. Для проверки выполните из
затронутой сети:

```sh
curl -i "https://${API_PROXY_DOMAIN}/api/v1/articles?api_key=default&limit=1"
```

После ответа 200 укажите на фронтенде базовый URL
`https://<домен-прокси>/api/v1`. При использовании гостевых cookie выбирайте
поддомен сайта. На стороне Vercel соединения приходят с IP прокси, поэтому
проверьте ограничения частоты запросов при общей нагрузке пользователей.
