# Глобальный администратор и API для CRM

Глобальный администратор — учётная запись платформы, а не сайта. Он не входит
через `/api/v1`, не пишет статьи от своего имени и не виден в списках
пользователей. Он управляет всеми сайтами через `/api/admin/v1`; заголовок
`X-Site-Key` там не нужен, сайт указывается в пути: `/sites/{site_key}/...`.

Токены не пересекаются: токен администратора не действует на `/api/v1`, а
токен пользователя сайта (даже с ролью `admin`) не действует на `/api/admin/v1`.

## Создание первого администратора

Через API создать администратора нельзя. Оператор запускает команду с доступом
к БД (команда сама применяет миграции):

```sh
# Локально: DATABASE_URL из .env
go run ./cmd/admin create -email admin@example.org
# Пароль генерируется и печатается один раз; свой можно задать в ADMIN_PASSWORD.

# Сброс пароля (все токены этого администратора отзываются):
go run ./cmd/admin reset-password -email admin@example.org
```

Для production нужен `DATABASE_URL_UNPOOLED` из Vercel. Не сохраняйте его в
репозитории; например, скачайте во временный файл и удалите после запуска:

```sh
T=$(mktemp) && npx vercel@63.1.2 env pull "$T" --environment=production --yes \
  && set -a && . "$T" && set +a && rm -f "$T" \
  && go run ./cmd/admin create -email admin@example.org
```

## Маршруты

| Метод и путь | Что делает |
| --- | --- |
| `POST /auth/login` | Вход по email и паролю, токен на 12 часов (refresh нет) |
| `POST /auth/logout` | Отзыв текущего токена |
| `GET /me` | Текущий администратор |
| `GET, POST /sites` | Список и создание сайтов |
| `GET, PATCH /sites/{site_key}` | Сайт; изменение `name` и `public_url` |
| `GET, POST /sites/{site_key}/users` | Пользователи (фильтры `role`, `q`); создание с любой ролью |
| `GET, PATCH, DELETE /sites/{site_key}/users/{user_id}` | Профиль, роль, пароль; удаление вместе с контентом |
| `GET, POST /sites/{site_key}/articles` | Статьи всех авторов, включая черновики; создание от имени `author_id` |
| `GET, PATCH, DELETE /sites/{site_key}/articles/{article_id}` | Любая статья; `PATCH`/`DELETE` требуют `If-Match` |
| `GET, POST /sites/{site_key}/invites` | Приглашения сайта; создание возвращает `token` один раз |
| `DELETE /sites/{site_key}/invites/{invite_id}` | Отзыв приглашения |

Все пути выше — относительно `/api/admin/v1`. Схемы и коды ответов — в
[openapi.yaml](../api/openapi.yaml) и Swagger UI (`/docs`, тег Admin).

Новый пароль пользователя отзывает все его сессии. Новая роль действует сразу.

## Приглашения редакторов

1. CRM создаёт приглашение:
   ```sh
   curl -X POST "$API/api/admin/v1/sites/med-assistant/invites" \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
     -d '{"role":"author","email":"editor@example.org","expires_in_hours":72}'
   ```
   `role` по умолчанию `author`, срок по умолчанию 72 часа (1..720). `email`
   необязателен; если он указан, зарегистрироваться можно только с этим адресом.
2. CRM строит ссылку на страницу регистрации сайта, например
   `https://med.example.org/register?invite=<token>`. В API хранится только
   SHA-256 токена, поэтому показать ссылку повторно нельзя: создайте новое
   приглашение.
3. Страница регистрации может проверить приглашение:
   `GET /api/v1/invites/{token}` с `X-Site-Key` → роль, email и срок; `404`, если
   приглашение использовано, истекло, отозвано или выдано для другого сайта.
4. Фронт регистрирует пользователя как обычно, добавив `invite_token`:
   ```json
   {"username":"editor","email":"editor@example.org","password":"…","invite_token":"<token>"}
   ```
   Пользователь сразу получает роль из приглашения, приглашение расходуется.
   Без `invite_token` регистрация по-прежнему даёт роль `user`.

## Тестовое наполнение

Миграция `005_demo_articles.sql` сразу создаёт на сайте `default` пользователя
`demo_author` (роль author, войти нельзя, пока администратор не задаст пароль)
и 10 опубликованных статей с тегом `demo` и просмотрами. Удалить их:
`DELETE /api/admin/v1/sites/default/users/{id}` для `demo_author` — статьи удалятся каскадно.

`cmd/seed` наполняет сайт демо-контентом через HTTP API и заодно проверяет
основные сценарии: вход администратора, создание пользователей, вход на сайт,
статьи, черновики, просмотры, лайки, комментарии, теги и статистику.

```sh
API_URL=https://blog-api-zeta-green.vercel.app \
ADMIN_EMAIL=admin@example.org ADMIN_PASSWORD='…' \
go run ./cmd/seed -site default
```

Создаются пользователи `demo_author` (author) и `demo_reader` (user) с email
`@demo.example` и шесть статей с тегом `demo` (одна — черновик). Повторный запуск
безопасен: пароли демо-аккаунтов меняются, существующие статьи не дублируются.
Удалить демо-контент: `DELETE /api/admin/v1/sites/{site_key}/users/{id}` для обоих
демо-пользователей.
