import assert from 'node:assert/strict';
import test from 'node:test';
import proxy from '../proxy.mjs';

test('passes the original version through the private transport header', () => {
  const response = proxy(new Request('https://example.org/api/v1/articles/1', {
    method: 'PATCH',
    headers: { 'if-match': '"1"', 'x-blog-if-match': '"999"', authorization: 'Bearer test' },
  }));
  assert.equal(response.headers.get('x-middleware-request-x-blog-if-match'), '"1"');
  assert.equal(response.headers.get('x-middleware-request-if-match'), null);
  assert.equal(response.headers.get('x-middleware-request-authorization'), 'Bearer test');
});

test('strips spoofed version when If-Match is absent', () => {
  const response = proxy(new Request('https://example.org/api/v1/articles/1', {
    method: 'DELETE', headers: { 'x-blog-if-match': '"1"' },
  }));
  assert.equal(response.headers.get('x-middleware-request-x-blog-if-match'), null);
});

test('does not change conditional read headers', () => {
  const response = proxy(new Request('https://example.org/api/v1/articles/1', {
    headers: { 'if-match': '"1"', 'x-blog-if-match': '"999"' },
  }));
  assert.equal(response.headers.get('x-middleware-request-if-match'), '"1"');
  assert.equal(response.headers.get('x-middleware-request-x-blog-if-match'), null);
});
