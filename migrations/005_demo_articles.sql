-- Demo content for the default site so the API can be tried right after deploy.
-- The demo author cannot log in (the password hash matches no password); a platform admin
-- can set a password via PATCH /api/admin/v1/sites/default/users/{id}, and cmd/seed does that.
-- Remove the demo with DELETE /api/admin/v1/sites/default/users/{id}: articles cascade.
INSERT INTO users(site_id,username,email,password_hash,display_name,bio,role)
SELECT id,'demo_author','demo_author@demo.example','!','Демо-автор','Автор демонстрационных статей.','author'
FROM sites WHERE key='default'
ON CONFLICT DO NOTHING;

CREATE TEMP TABLE demo(n int,title text,slug text,description text,content text,tags text[]) ON COMMIT DROP;
INSERT INTO demo VALUES
 (1,'Добро пожаловать в блог','welcome','Первая демонстрационная статья.',E'# Добро пожаловать\n\nЭто демонстрационная статья. Она показывает, что API хранит и отдаёт публикации.','{demo,news}'::text[]),
 (2,'Как устроен этот API','how-the-api-works','Коротко об архитектуре.',E'## Архитектура\n\nGo-монолит и PostgreSQL. Сайты изолированы заголовком `X-Site-Key`, авторизация — Bearer-токены.','{demo,go}'),
 (3,'Пять советов по написанию статей','five-writing-tips','Пишите понятно и коротко.',E'1. Начинайте с главного.\n2. Один абзац — одна мысль.\n3. Используйте заголовки.\n4. Добавляйте примеры.\n5. Перечитывайте перед публикацией.','{demo,writing}'),
 (4,'Markdown: шпаргалка','markdown-cheatsheet','Основные элементы разметки.',E'**Жирный**, *курсив*, `код`.\n\n- список\n- ещё пункт\n\n> Цитата','{demo,writing}'),
 (5,'Здоровый сон: базовые правила','healthy-sleep-basics','Режим, проветривание и меньше экранов вечером.',E'Ложитесь и вставайте в одно время, проветривайте спальню, ограничьте экраны за час до сна.\n\n*Текст демонстрационный и не является медицинской рекомендацией.*','{demo,health}'),
 (6,'Сколько воды нужно пить','water-intake','Почему универсальной нормы нет.',E'Потребность в воде зависит от веса, активности и климата. Ориентируйтесь на жажду и цвет мочи.\n\n*Текст демонстрационный и не является медицинской рекомендацией.*','{demo,health}'),
 (7,'Ходьба как ежедневная тренировка','daily-walking','Простая активность без спортзала.',E'Ходьба в среднем темпе 30 минут в день — доступный способ поддерживать выносливость.\n\n*Текст демонстрационный и не является медицинской рекомендацией.*','{demo,health,sport}'),
 (8,'Как читать состав продуктов','reading-food-labels','Что смотреть на упаковке в первую очередь.',E'Ингредиенты перечислены по убыванию доли. Обращайте внимание на сахар, соль и размер порции.','{demo,nutrition}'),
 (9,'Цифровой детокс на выходных','digital-detox','Отдых от уведомлений.',E'Отключите уведомления, оставьте телефон в другой комнате, запланируйте занятие офлайн.','{demo,lifestyle}'),
 (10,'Чек-лист перед визитом к врачу','doctor-visit-checklist','Что подготовить заранее.',E'- Список жалоб и когда они начались\n- Принимаемые лекарства\n- Результаты прошлых анализов\n- Вопросы, которые хотите задать','{demo,health}');

INSERT INTO tags(name) SELECT DISTINCT unnest(tags) FROM demo ON CONFLICT(name) DO NOTHING;

CREATE TEMP TABLE demo_inserted ON COMMIT DROP AS
WITH author AS (
 SELECT u.id,u.site_id FROM users u JOIN sites s ON s.id=u.site_id WHERE s.key='default' AND u.username='demo_author'
), ins AS (
 INSERT INTO articles(site_id,author_id,title,slug,description,content,status,created_at,updated_at,published_at)
 SELECT a.site_id,a.id,d.title,d.slug,d.description,d.content,'published',
  now()-(11-d.n)*interval '1 day',now()-(11-d.n)*interval '1 day',now()-(11-d.n)*interval '1 day'
 FROM demo d CROSS JOIN author a
 ON CONFLICT DO NOTHING
 RETURNING id,slug
) SELECT * FROM ins;

INSERT INTO article_tags(article_id,tag_id)
SELECT i.id,t.id FROM demo_inserted i JOIN demo d ON d.slug=i.slug CROSS JOIN LATERAL unnest(d.tags) AS x(name) JOIN tags t ON t.name=x.name
ON CONFLICT DO NOTHING;

-- Different view counts make sort=views and sort=popularity return distinct orders.
INSERT INTO article_daily_views(article_id,day,views)
SELECT i.id,current_date,(d.n*37)%101+5 FROM demo_inserted i JOIN demo d ON d.slug=i.slug
ON CONFLICT DO NOTHING;
