import { next } from '@vercel/functions';

export default function proxy(request) {
  const headers = new Headers(request.headers);
  // Discard client-supplied transport headers; only If-Match is authoritative.
  headers.delete('x-blog-if-match');
  if (request.method === 'PATCH' || request.method === 'DELETE') {
    const version = headers.get('if-match');
    if (version !== null) {
      headers.set('x-blog-if-match', version);
      headers.delete('if-match');
    }
  }
  // The API compares the original version against PostgreSQL. Hiding the
  // standard header here prevents the CDN comparing it with the new ETag.
  return next({ request: { headers } });
}
