# Развёртывание API на Render

[Создать Blueprint из репозитория](https://render.com/deploy?repo=https://github.com/IPodymov/blog-api). Выберите ветку main. Введите адрес существующей базы Neon в DATABASE_URL, прямое подключение к той же базе в DATABASE_URL_UNPOOLED, действующий VISITOR_SECRET и публичный адрес сайта в PUBLIC_SITE_URL.

Web Service запускает миграции перед стартом API и принимает запросы на порту из PORT. Render проверяет готовность через /ready. Отдельный платный Cron Job запускает обслуживание ежедневно в 03:00 UTC.

После деплоя проверьте /ready на домене из карточки Web Service. Укажите этот домен с https:// в base_url окружения Postman. При использовании Caddy задайте API_UPSTREAM_HOST равным имени хоста без схемы.
