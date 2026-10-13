"""Generate deterministic OpenAPI 3.1; JSON is a subset of YAML 1.2."""
import json
from pathlib import Path


def ref(n): return {'$ref': '#/components/schemas/' + n}


def string(min=0, max=None, **kw):
    s = {'type': 'string', 'minLength': min, **kw}
    if max is not None: s['maxLength'] = max
    return s


def integer(min=0, **kw): return {'type': 'integer', 'format': 'int64', 'minimum': min, **kw}


def obj(props, required=None, **kw): return {'type': 'object', 'additionalProperties': False, 'properties': props,
                                             'required': required if required is not None else list(props), **kw}


def arr(s, max=None):
    a = {'type': 'array', 'items': s}
    if max is not None: a['maxItems'] = max
    return a


def nullable(s): return {'anyOf': [s, {'type': 'null'}]}


S = {}
S['User'] = obj({'id': integer(1), 'username': string(3, 32, pattern='^[a-z0-9_]{3,32}$', example='alice'),
                 'display_name': string(0, 100, example='Алиса'),
                 'avatar_url': string(0, 2048, example='https://example.org/avatar.png'), 'bio': string(0, 2000),
                 'created_at': string(format='date-time')})
S['OwnUser'] = obj({**S['User']['properties'], 'email': string(3, 254, format='email', example='alice@example.org'),
                    'role': string(enum=['user', 'admin'])})
metric = {'published_articles': integer(), 'draft_articles': integer(), 'views_all_time': integer(),
          'likes_current': integer(), 'comments_current': integer()}

S['PublicMetrics'] = obj({k: v for k, v in metric.items() if k != 'draft_articles'})
S['Profile'] = obj({**S['User']['properties'], 'statistics': ref('PublicMetrics')})
input_article = {'title': string(1, 200, example='Первый пост'),
                 'slug': string(1, 200, pattern='^[a-z0-9]+(-[a-z0-9]+)*$', example='first-post'),
                 'description': string(0, 500, example='Краткое описание'), 'content': string(1, 100000,
                                                                                              description='Markdown; клиент отключает raw HTML и очищает результат перед отображением.',
                                                                                              example='# Привет\nМой первый пост.'),
                 'tags': arr(string(1, 40, pattern='^[a-z0-9]+(-[a-z0-9]+)*$'), 10),
                 'status': string(enum=['draft', 'published'], default='draft')}
S['ArticleInput'] = obj(input_article, ['title', 'slug', 'content'])
S['ArticlePatch'] = obj(input_article, [], minProperties=1,
                        description='Отсутствующее поле сохраняет значение; null запрещён. Пустая строка очищает description, [] очищает tags. title/content/slug не могут быть пустыми.')
S['Article'] = obj({'id': integer(1), 'author': ref('User'), **input_article, 'version': integer(1),
                    'created_at': string(format='date-time'), 'updated_at': string(format='date-time'),
                    'published_at': nullable(string(format='date-time')), 'views': integer(), 'likes': integer(),
                    'comments': integer(),
                    'popularity': integer(description='views + 5*likes + 3*comments, показатели за всё время/текущие'),
                    'liked_by_me': {'type': 'boolean'}, 'canonical_url': nullable(
        string(format='uri', example='https://blog.example.org/articles/first-post'))})
S['ProfilePatch'] = obj({'display_name': string(0, 100),
                         'avatar_url': string(0, 2048, description='HTTPS URL; пустая строка очищает значение.'),
                         'bio': string(0, 2000)}, [], minProperties=1,
                        description='Отсутствующее поле сохраняет значение, null запрещён, пустая строка очищает поле.')
S['CommentInput'] = obj({'content': string(1, 5000, example='Отличная статья!'), 'parent_id': nullable(integer(1))},
                        ['content'],
                        description='Ответ разрешён только корневому неудалённому комментарию этой статьи; максимальная глубина 1.')
S['CommentPatch'] = obj({'content': string(1, 5000, example='Спасибо за статью!')},
                        description='null и пустая строка запрещены.')
S['Comment'] = obj(
    {'id': integer(1), 'article_id': integer(1), 'author': ref('User'), 'parent_id': nullable(integer(1)),
     'depth': integer(0, maximum=1), 'content': string(0, 5000), 'deleted': {'type': 'boolean'},
     'created_at': string(format='date-time'), 'updated_at': string(format='date-time')})
S['Tokens'] = obj(
    {'access_token': string(43, 43), 'refresh_token': string(43, 43), 'token_type': string(enum=['Bearer']),
     'expires_in': integer(1, example=900)})
S['Register'] = obj({'username': S['User']['properties']['username'], 'email': S['OwnUser']['properties']['email'],
                     'password': string(12, 72, writeOnly=True, description='12..72 байта UTF-8, bcrypt cost 12.',
                                        example='example-password-2026')})
S['Login'] = obj({'email': S['OwnUser']['properties']['email'],
                  'password': string(1, 72, writeOnly=True, example='example-password-2026')})
S['Refresh'] = obj({'refresh_token': string(43, 43, writeOnly=True,
                                            description='Случайный непрозрачный токен; старый токен после ротации использовать нельзя.')})
S['Error'] = obj({'error': obj(
    {'code': string(example='validation_failed'), 'message': string(example='Некорректные данные'), 'details': nullable(
        {'type': 'object', 'additionalProperties': {'type': 'string'}, 'example': {'title': 'Недопустимая длина'}}),
     'request_id': string(example='qZd0pP6HAZChOcQM_aMmEDVc2qdFKGBWA5jdU7hjHxI')})})
S['Like'] = obj({'likes': integer(), 'liked_by_me': {'type': 'boolean'}})
S['View'] = obj(
    {'counted': {'type': 'boolean', 'description': 'false: дедупликация, просмотр автора или администратора.'}})
S['Day'] = obj({'date': string(format='date', example='2026-10-13'), 'views': integer()})
S['ArticleStat'] = obj({'article': ref('Article'), 'views_period': integer()})
period_props = {'date_from': string(format='date'), 'date_to': string(format='date'), 'timezone': string(enum=['UTC']),
                'views_period': integer(), 'daily': arr(ref('Day'))}
S['Statistics'] = obj({**metric, **period_props, 'top_articles': arr(ref('ArticleStat'), 5)},
                      description='Все собственные существующие статьи, включая черновики и снятые с публикации. Текущие лайки/комментарии; только просмотры имеют историю.')
S['ArticleStatistics'] = obj(
    {'article_id': integer(1), 'views_all_time': integer(), 'likes_current': integer(), 'comments_current': integer(),
     **period_props})
S['Health'] = obj({'status': string(enum=['ok', 'ready'])})
for name, item in [('ArticlePage', ref('Article')), ('ArticleStatPage', ref('ArticleStat')),
                   ('CommentPage', ref('Comment')), ('TagPage', string(1, 40))]:
    S[name] = obj({'items': arr(item), 'page': integer(1), 'page_size': integer(1, maximum=100), 'total': integer()})
params = {}


def param(name, schema, desc='', where='query', required=False): return {'name': name, 'in': where,
                                                                         'required': required, 'description': desc,
                                                                         'schema': schema}


params['Page'] = param('page', integer(1, maximum=100000, default=1), 'Номер страницы; offset pagination.')
params['PageSize'] = param('page_size', integer(1, maximum=100, default=20))
params['Sort'] = param('sort',
                       string(enum=['published_at', 'views', 'popularity', 'created_at'], default='published_at'),
                       'Убывание; ID DESC — стабильное разрешение равенств. При живых изменениях состав страниц может меняться.')
params['StatSort'] = param('sort', string(enum=['views_period', 'views', 'likes', 'comments', 'published_at'],
                                          default='views_period'), 'Убывание, затем ID DESC.')
params['From'] = param('date_from', string(format='date'), 'Включительно, UTC; по умолчанию сегодня минус 29 дней.')
params['To'] = param('date_to', string(format='date'),
                     'Включительно, UTC; по умолчанию сегодня. Не будущая дата. Максимум MAX_STATISTICS_DAYS (по умолчанию 366) дней.')
params['IfMatch'] = param('If-Match', string(pattern='^"[1-9][0-9]*"$', example='"1"'),
                          'Обязательная версия из ETag. Несовпадение: 412; отсутствие: 428. DELETE тоже защищён от потери изменений.',
                          'header', True)
params['Author'] = param('author_id', integer(1))
params['Tag'] = param('tag', string(0, 40), 'Один тег; точное совпадение.')
params['Search'] = param('q', string(0, 200),
                         'PostgreSQL full text simple; заголовок, описание, Markdown. Лимит 200 байт.')
params['Status'] = param('status', string(enum=['draft', 'published']),
                         'Только свои статьи; отсутствие означает оба статуса.')


def pref(n): return {'$ref': '#/components/parameters/' + n}


pages = [pref('Page'), pref('PageSize')]
period = [pref('From'), pref('To')]
responses = {}
status_desc = {400: 'Некорректный JSON, неизвестное поле или форма запроса.',
               401: 'Необходима авторизация / токен или учётные данные недействительны.',
               403: 'Недостаточно прав или Origin не разрешён.', 404: 'Ресурс не найден; чужие черновики также 404.',
               405: 'Метод не поддерживается.', 409: 'Email, username или slug уже используется.',
               412: 'Версия статьи изменилась.', 413: 'Превышен лимит тела 512 KiB.',
               415: 'Ожидается application/json.', 422: 'Ошибка валидации поля или диапазона дат.',
               428: 'Отсутствует If-Match.', 429: 'Превышен лимит запросов.', 500: 'Внутренняя ошибка.',
               503: 'PostgreSQL недоступен.'}
for code, d in status_desc.items():
    responses['E' + str(code)] = {'description': d, 'content': {'application/json': {'schema': ref('Error'),
                                                                                     'example': {'error': {'code': {
                                                                                         401: 'authentication_required',
                                                                                         404: 'not_found',
                                                                                         412: 'version_conflict',
                                                                                         422: 'validation_failed',
                                                                                         429: 'rate_limited'}.get(code,
                                                                                                                  'request_failed'),
                                                                                                           'message': d,
                                                                                                           'details': None,
                                                                                                           'request_id': 'request-example'}}}}}
responses['E429']['headers'] = {'Retry-After': {'schema': string(), 'description': '60 секунд'}}
paths = {}


def op(path, method, oid, tag, summary, response=None, body=None, secure=False, parameters=None, status=200, desc='',
       errors=None):
    p = '/api/v1' + path if not path.startswith(('/health', '/ready', '/docs', '/openapi')) else path
    ps = list(parameters or [])
    import re
    for name in re.findall(r'{([^}]+)}', path): ps.insert(0,
                                                          param(name, string(1, 200) if name == 'slug' else integer(1),
                                                                '', 'path', True))
    r = {str(status): {'description': 'Успешно.'}}
    if response: r[str(status)]['content'] = {'application/json': {'schema': ref(response)}}
    err = set(errors or []) | {403, 413, 429, 500}
    if secure: err.add(401)
    if '{' in path: err.update([404, 422])
    if body: err.update([400, 413, 415, 422])
    if parameters: err.add(422)
    for code in sorted(err): r[str(code)] = {'$ref': '#/components/responses/E' + str(code)}
    o = {'operationId': oid, 'tags': [tag], 'summary': summary, 'description': desc or summary, 'parameters': ps,
         'security': [{'bearerAuth': []}] if secure else [{}, {'bearerAuth': []}], 'responses': r}
    if body: o['requestBody'] = {'required': True, 'content': {'application/json': {'schema': ref(body)}}}
    paths.setdefault(p, {})[method] = o
    return o


op('/auth/register', 'post', 'register', 'Auth', 'Регистрация пользователя', 'Tokens', 'Register', status=201,
   errors=[409],
   desc='Роль всегда user. Возвращает токены только в этом приватном ответе. Rate limit всех auth: 10 запросов/мин на сетевой адрес.')
op('/auth/login', 'post', 'login', 'Auth', 'Вход', 'Tokens', 'Login', errors=[401])
op('/auth/refresh', 'post', 'refreshTokens', 'Auth', 'Ротация refresh-токена', 'Tokens', 'Refresh', errors=[401],
   desc='Одноразовая ротация. Повторное использование использованного токена отзывает всё семейство, включая access-токены. Одновременные refresh-запросы клиент должен сериализовать. Абсолютный срок семейства не продлевается.')
op('/auth/logout', 'post', 'logout', 'Auth', 'Выход и отзыв семейства', body='Refresh', status=204,
   desc='Идемпотентно, включая неизвестный токен корректного формата. Отзывает также все access-токены семейства.')
op('/users/{user_id}', 'get', 'getPublicProfile', 'Users', 'Публичный профиль', 'Profile',
   desc='Email и роль отсутствуют. Счётчики только опубликованных статей.')
op('/users/{user_id}/articles', 'get', 'getAuthorArticles', 'Users', 'Опубликованные статьи автора', 'ArticlePage',
   parameters=pages + [pref('Sort'), pref('Tag'), pref('Search')])
op('/me', 'get', 'getMe', 'Users', 'Собственный профиль с email', 'OwnUser', secure=True)
op('/me', 'patch', 'updateMe', 'Users', 'Редактирование собственного профиля', 'OwnUser', 'ProfilePatch', secure=True)
op('/me/articles', 'get', 'getMyArticles', 'Users', 'Все собственные статьи', 'ArticlePage', secure=True,
   parameters=pages + [pref('Sort'), pref('Status'), pref('Tag'), pref('Search')])
op('/me/statistics', 'get', 'getMyStatistics', 'Statistics', 'Сводная статистика автора', 'Statistics', secure=True,
   parameters=period,
   desc='Все существующие свои статьи. Возвращает 5 самых просматриваемых за период, включая нулевые. Каждый день периода присутствует, в том числе с 0. Лайки и комментарии текущие, не события периода.')
op('/me/statistics/articles', 'get', 'getMyArticleStatistics', 'Statistics', 'Показатели собственных статей',
   'ArticleStatPage', secure=True, parameters=pages + [pref('StatSort')] + period)
op('/articles', 'get', 'listArticles', 'Articles', 'Публичный список публикаций', 'ArticlePage',
   parameters=pages + [pref('Sort'), pref('Author'), pref('Tag'), pref('Search')])
o = op('/articles', 'post', 'createArticle', 'Articles', 'Создать статью', 'Article', 'ArticleInput', secure=True,
       status=201, errors=[409])
o['responses']['201']['headers'] = {'Location': {'schema': string()}, 'ETag': {'schema': string(example='"1"')}}
for path, oid in [('/articles/{article_id}', 'getArticle'), ('/articles-by-slug/{slug}', 'getArticleBySlug')]:
    o = op(path, 'get', oid, 'Articles', 'Получить статью', 'Article',
           desc='GET не регистрирует просмотр. По ID: черновик доступен автору и администратору. По slug: только опубликованная статья. canonical_url строится из PUBLIC_SITE_URL + /articles/{slug}; у черновика null.')
    o['responses']['200']['headers'] = {'ETag': {'schema': string(example='"1"')}}
o = op('/articles/{article_id}', 'patch', 'updateArticle', 'Articles', 'Изменить статью', 'Article', 'ArticlePatch',
       secure=True, parameters=[pref('IfMatch')], errors=[409, 412, 428],
       desc='Автор либо администратор. Проверка версии атомарна. Снятие публикации сохраняет просмотры, лайки и комментарии для личной аналитики; из публичной статистики статья исчезает.')
o['responses']['200']['headers'] = {'ETag': {'schema': string(example='"2"')}}
op('/articles/{article_id}', 'delete', 'deleteArticle', 'Articles', 'Удалить статью', secure=True,
   parameters=[pref('IfMatch')], status=204, errors=[412, 428],
   desc='Автор либо администратор. Физическое удаление каскадно удаляет комментарии, лайки, связи тегов, дедупликацию и агрегаты просмотров; все показатели автора пересчитываются сразу.')
o = op('/articles/{article_id}/views', 'post', 'registerView', 'Views', 'Зарегистрировать фактическое открытие', 'View',
       desc='Без тела. Только опубликованная статья. Автор и администратор: counted=false. Один учитываемый просмотр на посетителя/статью за VIEW_INTERVAL (по умолчанию 30 минут); отклонённый дубль окно не продлевает. Конкурентные запросы защищены транзакцией. Для авторизованного посетителя используется ID аккаунта, для гостя — подписанный сервером случайный cookie blog_visitor. Невалидный cookie заменяется. IP не хранится в аналитике. Лимит 60 запросов/мин на сетевой адрес. События просмотров и уникальные посетители не равнозначны; уникальные посетители не рассчитываются.')
o['parameters'].append(param('blog_visitor', string(),
                             'Не задавайте вручную. Сервер выдаёт подписанный непрозрачный cookie: HttpOnly, SameSite=Lax, Path=/api/v1, Max-Age=31536000, Secure по конфигурации (true в production). Cross-origin same-site клиент использует credentials: include и разрешённый CORS origin. Cross-site cookie может блокироваться; такой хостинг не поддерживает точную гостевую дедупликацию. Удаление cookie и смена браузера создают нового посетителя. Первые параллельные запросы без cookie также могут посчитаться отдельно.',
                             'cookie'))
o['responses']['200']['headers'] = {
    'Set-Cookie': {'schema': string(), 'description': 'Для нового/невалидного гостевого cookie.'}}
op('/articles/{article_id}/statistics', 'get', 'getArticleStatistics', 'Statistics', 'Подробная статистика статьи',
   'ArticleStatistics', secure=True, parameters=period,
   desc='Только автор или администратор, включая черновики. UTC, границы дат включительны. Все дни с нулями; просмотры all-time и за период, лайки/комментарии текущие.')
op('/articles/{article_id}/comments', 'get', 'listComments', 'Comments', 'Комментарии статьи', 'CommentPage',
   parameters=pages,
   desc='Публикация доступна всем; черновик только автору и администратору. Плоский список ID ASC; parent_id связывает ответы. Удалённые комментарии представлены tombstone: content пустой, deleted=true; в счётчик не входят.')
op('/articles/{article_id}/comments', 'post', 'createComment', 'Comments', 'Добавить комментарий или ответ', 'Comment',
   'CommentInput', secure=True, status=201,
   desc='Только к опубликованным статьям. Максимальная глубина ответов 1; parent_id должен указывать на неудалённый корневой комментарий этой же статьи.')
op('/comments/{comment_id}', 'patch', 'updateComment', 'Comments', 'Изменить свой комментарий', 'Comment',
   'CommentPatch', secure=True, desc='Автор комментария или администратор; удалённый комментарий изменять нельзя.')
op('/comments/{comment_id}', 'delete', 'deleteComment', 'Comments', 'Удалить свой комментарий', secure=True, status=204,
   desc='Автор комментария или администратор. Soft delete с очисткой текста, ответы сохраняются, повторный DELETE существующего tombstone идемпотентен.')
op('/articles/{article_id}/like', 'put', 'likeArticle', 'Likes', 'Поставить лайк', 'Like', secure=True,
   desc='Только опубликованная статья. Идемпотентно: уникальная пара user/article.')
op('/articles/{article_id}/like', 'delete', 'unlikeArticle', 'Likes', 'Снять лайк', 'Like', secure=True,
   desc='Только опубликованная статья. Идемпотентно, даже если лайка нет.')
op('/tags', 'get', 'listTags', 'Tags', 'Теги опубликованных статей', 'TagPage', parameters=pages,
   desc='Алфавитный порядок name ASC; теги только черновиков не раскрываются.')
op('/health', 'get', 'health', 'Operations', 'Проверка процесса', 'Health')
op('/ready', 'get', 'ready', 'Operations', 'Проверка PostgreSQL', 'Health', errors=[503])
for path, oid, media in [('/docs', 'getDocs', 'text/html'), ('/openapi.yaml', 'getOpenAPI', 'application/yaml')]:
    o = op(path, 'get', oid, 'Operations', 'Документация API');
    o['responses']['200']['content'] = {media: {'schema': string()}}
root = {'openapi': '3.1.0', 'jsonSchemaDialect': 'https://json-schema.org/draft/2020-12/schema',
        'info': {'title': 'Multi-user Blog API', 'version': '1.0.0',
                 'description': 'Модульный монолит Go/PostgreSQL. Даты UTC, int64 ID. Все ошибки имеют request_id. Bearer access-токены и refresh в JSON; cookie не используется для авторизации. Общий лимит 300 запросов/мин, auth 10, views 60 на сетевой адрес. Тело до 512 KiB. Неизвестные поля тела запрещены. PATCH null запрещён. offset pagination с лимитом 100. Счётчики обновляются синхронно после commit. Не хранится историческая динамика лайков/комментариев. Нет метрики уникальных посетителей. На любом маршруте может быть 401, если передан невалидный Authorization.'},
        'servers': [{'url': 'http://localhost:8080', 'description': 'Локальный запуск'}],
        'tags': [{'name': x, 'description': x + ' operations'} for x in
                 ['Auth', 'Users', 'Articles', 'Views', 'Statistics', 'Comments', 'Likes', 'Tags', 'Operations']],
        'paths': paths, 'components': {'schemas': S, 'parameters': params, 'responses': responses, 'securitySchemes': {
        'bearerAuth': {'type': 'http', 'scheme': 'bearer',
                       'description': 'Непрозрачный 256-битный access-токен, TTL по умолчанию 15 минут; БД хранит только SHA-256.'}}}}
for path, methods in paths.items():
    for o in methods.values():
        o['responses'].setdefault('405', {'$ref': '#/components/responses/E405'})
        o['responses'].setdefault('401', {'$ref': '#/components/responses/E401'})
Path('openapi.yaml').write_text(json.dumps(root, ensure_ascii=False, indent=2) + '\n')


# Complete operation examples, validated by the standard OpenAPI linter.
def sample(s):
    if '$ref' in s: return sample(S[s['$ref'].split('/')[-1]])
    if 'example' in s: return s['example']
    if 'anyOf' in s: return None
    if 'enum' in s: return s.get('default', s['enum'][0])
    t = s.get('type')
    if t == 'object': return {k: sample(v) for k, v in s.get('properties', {}).items()}
    if t == 'array': return []
    if t == 'boolean': return False
    if t == 'integer': return s.get('minimum', 0)
    if t == 'string':
        if s.get('format') == 'date': return '2026-10-13'
        if s.get('format') == 'date-time': return '2026-10-13T12:00:00Z'
        if s.get('format') == 'email': return 'alice@example.org'
        if s.get('format') == 'uri': return 'https://blog.example.org'
        return 'a' * max(s.get('minLength', 0), 1)
    return None


for p, methods in paths.items():
    for operation in methods.values():
        if 'requestBody' in operation:
            media = operation['requestBody']['content']['application/json'];
            media['example'] = sample(media['schema'])
        for response in operation['responses'].values():
            if '$ref' in response: continue
            for media in response.get('content', {}).values():
                if media.get('schema', {}).get('$ref'): media['example'] = sample(media['schema'])
Path('openapi.yaml').write_text(json.dumps(root, ensure_ascii=False, indent=2) + '\n')
