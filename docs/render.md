# Бесплатный Web Service на Render

В Render Dashboard выберите New → Web Service и подключите репозиторий IPodymov/blog-api. Выберите ветку main, среду Docker, регион Frankfurt и тип инстанса Free.

В поле Docker Command укажите `/bin/sh -c '/app/migrate && exec /app/api'`, в Health Check Path — `/ready`. Порт передаёт Render через PORT.

В Environment укажите DATABASE_URL существующей базы Neon, прямое подключение к той же базе в DATABASE_URL_UNPOOLED, действующий VISITOR_SECRET и публичный адрес сайта в PUBLIC_SITE_URL.

На бесплатном инстансе отдельное задание по расписанию не создаётся. Для очистки истёкших сессий и записей дедупликации доступна команда `go run ./cmd/maintenance` с окружением целевой базы.

После деплоя проверьте /ready на домене из карточки Web Service. Укажите этот домен с https:// в base_url окружения Postman. При использовании Caddy задайте API_UPSTREAM_HOST равным имени хоста без схемы.
