"""Smoke test a running stack; creates disposable accounts/article, deletes the article.
Usage: python3 scripts/smoke.py http://localhost:8080
"""
import http.cookiejar
import json
import secrets
import sys
import urllib.error
import urllib.request

base = (sys.argv[1] if len(sys.argv) > 1 else 'http://localhost:8080').rstrip('/')
jar = http.cookiejar.CookieJar()
client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))


def request(method, path, data=None, token=None, version=None, status=200):
    headers = {}
    if data is not None: headers['Content-Type'] = 'application/json'
    if token: headers['Authorization'] = 'Bearer ' + token
    if version: headers['If-Match'] = version
    req = urllib.request.Request(base + path, data=None if data is None else json.dumps(data).encode(), headers=headers,
                                 method=method)
    try:
        r = client.open(req, timeout=15)
    except urllib.error.HTTPError as e:
        r = e
    raw = r.read();
    assert r.status == status, (method, path, r.status, raw[:200])
    return (json.loads(raw) if raw and 'application/json' in r.headers.get('Content-Type', '') else raw), r.headers


for path in ['/health', '/ready', '/docs', '/openapi.yaml']:
    data, _ = request('GET', path)
    if path == '/docs': assert b'SwaggerUIBundle' in data
    if path == '/openapi.yaml': assert json.loads(data)['openapi'] == '3.1.0'
suffix = secrets.token_hex(6)


def register(prefix):
    return request('POST', '/api/v1/auth/register',
                   {'username': prefix + suffix, 'email': prefix + suffix + '@example.org',
                    'password': secrets.token_urlsafe(24)}, status=201)[0]


owner = register('owner');
reader = register('reader')
me, _ = request('GET', '/api/v1/me', token=owner['access_token'])
article, h = request('POST', '/api/v1/articles',
                     {'title': 'Smoke test', 'slug': 'smoke-' + suffix, 'content': '# Smoke', 'tags': ['go']},
                     owner['access_token'], status=201)
p = '/api/v1/articles/' + str(article['id'])
request('GET', p, token=reader['access_token'], status=404)
article, h = request('PATCH', p, {'status': 'published'}, owner['access_token'], h['ETag'])
assert article['canonical_url'].endswith('/articles/smoke-' + suffix)
request('GET', p)
assert request('GET', p)[0]['views'] == 0
assert request('POST', p + '/views', token=owner['access_token'])[0]['counted'] is False
assert request('POST', p + '/views')[0]['counted'] is True
assert request('POST', p + '/views')[0]['counted'] is False
assert request('POST', p + '/views', token=reader['access_token'])[0]['counted'] is True
assert request('PUT', p + '/like', token=reader['access_token'])[0]['likes'] == 1
assert request('PUT', p + '/like', token=reader['access_token'])[0]['likes'] == 1
request('POST', p + '/comments', {'content': 'Smoke comment'}, reader['access_token'], status=201)
summary, _ = request('GET', '/api/v1/me/statistics', token=owner['access_token'])
assert summary['views_all_time'] == 2 and summary['likes_current'] == 1 and summary['comments_current'] == 1
profile, _ = request('GET', '/api/v1/users/' + str(me['id']))
assert 'email' not in profile and profile['statistics']['published_articles'] == 1
request('GET', p + '/statistics', token=reader['access_token'], status=403)
request('DELETE', p, token=owner['access_token'], version=h['ETag'], status=204)
for tokens in [reader, owner]: request('POST', '/api/v1/auth/logout', {'refresh_token': tokens['refresh_token']},
                                       status=204)
print(
    'Smoke passed: process/readiness/docs, registration, privacy, publication, views/cookies, likes/comments, statistics, deletion, logout.')
