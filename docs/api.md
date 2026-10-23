# Справочник API

Полный машинно-читаемый контракт находится в [`openapi.yaml`](../api/openapi.yaml),
интерактивная документация — по адресу `/docs`, сам OpenAPI — `/openapi.yaml`.
Все маршруты ниже, кроме `/health`, `/ready` и документации, начинаются с
`/api/v1`.

## Общие правила

- JSON-запросы отправляйте с `Content-Type: application/json`.
- Для закрытых операций передавайте `Authorization: Bearer <access_token>`.
  Access-токен получается при регистрации или входе. Refresh-токен передаётся
  только в теле запросов refresh/logout.
- Если у необязательного bearer-маршрута передан неверный `Authorization`,
  запрос всё равно завершится `401`; для анонимного вызова не отправляйте этот
  заголовок.
- `user_id`, `article_id` и `comment_id` — положительные `int64`.
- Неизвестные поля JSON запрещены. В `PATCH` нельзя передавать `null`;
  передавайте только поля, которые нужно изменить. Пустая строка может
  очищать поле, если это указано ниже.
- Максимальный размер тела — 512 KiB. Ошибки возвращаются JSON-объектом
  `{"error":{"code":"...","message":"...","details":{...}}}` с `request_id`.
- Списки используют нумерацию страниц с 1 и `page_size` до 100. Сортировка
  выполняется по убыванию, равные значения разрешаются по ID.
- Общий rate limit — 300 запросов/мин на сетевой адрес; auth — 10/мин,
  регистрация просмотров — 60/мин. Ограничитель локален для экземпляра API.

## Авторизация

| Метод и путь | Доступ | Параметры и тело |
| --- | --- | --- |
| `POST /api/v1/auth/register` | Анонимно | JSON: `username` (3–32 символа, `a-z`, `0-9`, `_`), `email` (до 254 символов), `password` (12–72 байта UTF-8). Роль нового пользователя — `user`. |
| `POST /api/v1/auth/login` | Анонимно | JSON: `email`, `password`. |
| `POST /api/v1/auth/refresh` | Анонимно | JSON: `refresh_token` (непрозрачная строка длиной 43 символа). Успешный вызов ротирует токен; старый повторно использовать нельзя. |
| `POST /api/v1/auth/logout` | Анонимно | JSON: `refresh_token`. Отзывает семейство токенов; успешный ответ `204` без тела. |

Регистрация возвращает `201`, вход и refresh — `200`. Ответ содержит
`access_token`, `refresh_token`, `token_type: "Bearer"` и `expires_in`
в секундах. При повторном использовании уже ротированного refresh-токена
отзывается всё семейство токенов. Клиенту следует сериализовать одновременные
refresh-запросы.

## Пользователи и профиль

| Метод и путь | Доступ | Параметры и тело |
| --- | --- | --- |
| `GET /api/v1/users/{user_id}` | Анонимно | `user_id` в пути. Публичный профиль без email. |
| `GET /api/v1/users/{user_id}/articles` | Анонимно | `user_id`; параметры списка ниже. Возвращает публикации автора. |
| `GET /api/v1/me` | Bearer | Текущий профиль, включая email. |
| `PATCH /api/v1/me` | Bearer | JSON с любыми полями: `display_name` (до 100 символов), `avatar_url` (HTTPS URL, до 2048 символов; `""` очищает), `bio` (до 2000 символов). |
| `GET /api/v1/me/articles` | Bearer | Собственные статьи, включая черновики; параметры списка ниже и `status`: `draft` или `published`. |
| `GET /api/v1/me/statistics` | Bearer | `date_from`, `date_to`; общая статистика автора за период. |
| `GET /api/v1/me/statistics/articles` | Bearer | `page`, `page_size`, `sort`, `date_from`, `date_to`; статистика по статьям автора. `sort`: `views_period`, `views`, `likes`, `comments`, `published_at`. |

## Статьи

| Метод и путь | Доступ | Параметры и тело |
| --- | --- | --- |
| `GET /api/v1/articles` | Анонимно | Публичные статьи; `page`, `page_size`, `sort`, `author_id`, `tag`, `q`. |
| `POST /api/v1/articles` | Bearer | JSON статьи (описание полей ниже). `status` по умолчанию `draft`. |
| `GET /api/v1/articles/{article_id}` | Анонимно* | `article_id` в пути. Черновик доступен только автору или администратору. |
| `GET /api/v1/articles-by-slug/{slug}` | Анонимно* | `slug` в пути (1–200 символов). Черновик доступен только автору или администратору. |
| `PATCH /api/v1/articles/{article_id}` | Bearer | `article_id`, обязательный `If-Match` и JSON с изменяемыми полями статьи. Только автор или администратор. |
| `DELETE /api/v1/articles/{article_id}` | Bearer | `article_id` и обязательный `If-Match`. Только автор или администратор; успешный ответ `204`. |

`*` Маршрут допускает анонимный доступ, но наличие Bearer-токена позволяет
проверить права на черновик. Если токен передан и он недействителен, вернётся
`401`.

Поля создания статьи:

| Поле | Тип и ограничения |
| --- | --- |
| `title` | Обязательная строка, 1–200 символов. |
| `slug` | Обязательная строка, 1–200 символов; строчные латинские буквы и цифры, сегменты разделены одиночным `-`. |
| `content` | Обязательная Markdown-строка, 1–100000 символов. |
| `description` | Необязательная строка, до 500 символов. |
| `tags` | Необязательный массив до 10 тегов; каждый до 40 символов, формат slug. |
| `status` | Необязательная строка: `draft` или `published`; по умолчанию `draft`. |

`PATCH` принимает любой непустой набор тех же полей; `null` запрещён. Markdown
не следует отображать как доверенный HTML: клиент должен отключать raw HTML и
очищать результат перед выводом.

Для `PATCH` и `DELETE` статьи передавайте точное значение `ETag`, полученное
при чтении статьи, в заголовке `If-Match`, включая кавычки, например
`If-Match: "3"`. Без заголовка ответ `428`, при устаревшей версии — `412`.
Это предотвращает перезапись чужих изменений.

### Параметры списков статей

| Параметр | Значение |
| --- | --- |
| `page` | Страница, целое число от 1 до 100000; по умолчанию `1`. |
| `page_size` | Размер страницы от 1 до 100; по умолчанию `20`. |
| `sort` | Для публичного списка, профиля и своих статей: `published_at`, `views`, `popularity` или `created_at`; по умолчанию `published_at`. |
| `author_id` | Положительный ID автора; применяется к `GET /api/v1/articles`. |
| `tag` | Один тег, точное совпадение; до 40 символов. |
| `q` | Полнотекстовый поиск по заголовку, описанию и Markdown; до 200 байт. |
| `status` | `draft` или `published`; только для списка собственных статей. Без параметра возвращаются оба статуса. |

## Просмотры, статистика, комментарии и лайки

| Метод и путь | Доступ | Параметры и тело |
| --- | --- | --- |
| `POST /api/v1/articles/{article_id}/views` | Анонимно | `article_id` в пути. Регистрирует фактический просмотр опубликованной статьи; сервер выдаёт и принимает подписанную cookie `blog_visitor` для дедупликации. |
| `GET /api/v1/articles/{article_id}/statistics` | Bearer | `article_id`, `date_from`, `date_to`. Доступно автору и администратору статьи. |
| `GET /api/v1/articles/{article_id}/comments` | Bearer | `article_id`, `page`, `page_size`. Список комментариев статьи. |
| `POST /api/v1/articles/{article_id}/comments` | Bearer | JSON: обязательный `content` (1–5000 символов); необязательный `parent_id` (положительный ID комментария или `null`) для ответа. |
| `PATCH /api/v1/comments/{comment_id}` | Bearer | JSON с обязательным новым `content` (1–5000 символов); редактировать может автор комментария. |
| `DELETE /api/v1/comments/{comment_id}` | Bearer | `comment_id`; удалить может автор комментария или администратор; успешный ответ `204`. |
| `PUT /api/v1/articles/{article_id}/like` | Bearer | `article_id`; поставить лайк. |
| `DELETE /api/v1/articles/{article_id}/like` | Bearer | `article_id`; снять лайк. |

`date_from` и `date_to` — календарные даты `YYYY-MM-DD` в UTC, обе границы
включаются. По умолчанию период — последние 30 дней, включая сегодня.
`date_to` не может быть в будущем; длина периода ограничена
`MAX_STATISTICS_DAYS` (по умолчанию 366).

Cookie `blog_visitor` создаётся сервером; вручную задавать её не нужно.
Для запросов с другого origin в рамках одного site используйте
`credentials: "include"` и настройте разрешённый CORS origin. Браузер может
блокировать cookie в cross-site сценариях.

## Теги и служебные маршруты

| Метод и путь | Доступ | Параметры и тело |
| --- | --- | --- |
| `GET /api/v1/tags` | Анонимно | `page`, `page_size`; теги опубликованных статей. |
| `GET /health` | Анонимно | Проверка процесса API. |
| `GET /ready` | Анонимно | Проверка доступности PostgreSQL; `503`, если БД недоступна. |
| `GET /docs` | Анонимно | Swagger UI. |
| `GET /openapi.yaml` | Анонимно | Полный контракт OpenAPI. |

Cron вызывает `GET /internal/maintenance` с `Authorization: Bearer <CRON_SECRET>`;
это служебная ручка и не часть публичного `/api/v1`. Она не требует пользовательского
access-токена.

## Примеры

Вход:

```sh
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.org","password":"example-password-2026"}'
```

Создание статьи:

```sh
curl -X POST http://localhost:8080/api/v1/articles \
  -H 'Authorization: Bearer <access_token>' \
  -H 'Content-Type: application/json' \
  -d '{"title":"Первый пост","slug":"first-post","content":"# Привет","tags":["go"]}'
```

Обновление статьи с контролем версии:

```sh
curl -X PATCH http://localhost:8080/api/v1/articles/1 \
  -H 'Authorization: Bearer <access_token>' \
  -H 'If-Match: "1"' \
  -H 'Content-Type: application/json' \
  -d '{"status":"published"}'
```
