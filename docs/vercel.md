# Vercel: API и PostgreSQL

Проект: `ipodymovs-projects/blog-api`, ID `prj_helVcKmAUCbwwH23qqbe7g64EjhH`.
Назначенный домен: `https://blog-api-zeta-green.vercel.app`.
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
| `PUBLIC_SITE_URL`       | HTTPS-адрес сайта, пока сайта нет — адрес API                |
| `CORS_ORIGINS`          | Разрешённые origins через запятую, без пути/завершающего `/` |
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
`python3 scripts/smoke.py https://blog-api-zeta-green.vercel.app` проверяет
реальные сценарии и создаёт два тестовых аккаунта; тестовую статью удаляет.

Проект подключён к `IPodymov/blog-api`, production-ветка `main`.
Для Git-автодеплоя нужно опубликовать текущие изменения в репозитории.
Каждая production-сборка применяет
миграции: используйте совместимые добавочные изменения схемы. Откат Vercel
не откатывает PostgreSQL.

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
cookie имеет `SameSite=Lax`. После появления сайта обновите `PUBLIC_SITE_URL`
и `CORS_ORIGINS` и выполните новый деплой. API не реализует HTML-страницы статей.

Официальная документация: [Go runtime](https://vercel.com/docs/functions/runtimes/go),
[Cron](https://vercel.com/docs/cron-jobs/manage-cron-jobs),
[заголовки платформы](https://vercel.com/docs/headers/request-headers).
