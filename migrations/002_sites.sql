-- Each blog site is an isolated tenant: users, articles, comments and likes never cross site boundaries.
CREATE TABLE sites (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 key text NOT NULL UNIQUE CHECK(key ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(key)<=64),
 name text NOT NULL DEFAULT '' CHECK(length(name)<=200),
 public_url text NOT NULL DEFAULT '' CHECK(public_url='' OR (public_url ~ '^https?://[^/?#@[:space:]]+(/[^?#[:space:]]*)?$' AND length(public_url)<=2048)),
 created_at timestamptz NOT NULL DEFAULT now()
);
-- Existing data belongs to the default site; empty public_url falls back to PUBLIC_SITE_URL.
INSERT INTO sites(key,name) VALUES('default','Default');

ALTER TABLE users ADD COLUMN site_id bigint REFERENCES sites;
UPDATE users SET site_id=(SELECT id FROM sites WHERE key='default');
ALTER TABLE users ALTER COLUMN site_id SET NOT NULL,
 DROP CONSTRAINT users_username_key, DROP CONSTRAINT users_email_key,
 ADD UNIQUE(site_id,username), ADD UNIQUE(site_id,email), ADD UNIQUE(id,site_id);

-- Composite FKs (x_id,site_id) make cross-site references impossible at the database level.
ALTER TABLE articles ADD COLUMN site_id bigint;
UPDATE articles a SET site_id=u.site_id FROM users u WHERE u.id=a.author_id;
ALTER TABLE articles ALTER COLUMN site_id SET NOT NULL,
 DROP CONSTRAINT articles_slug_key, DROP CONSTRAINT articles_author_id_fkey,
 ADD UNIQUE(site_id,slug), ADD UNIQUE(id,site_id),
 ADD FOREIGN KEY(author_id,site_id) REFERENCES users(id,site_id) ON DELETE CASCADE;
DROP INDEX articles_publication;
CREATE INDEX articles_publication ON articles(site_id,published_at DESC,id DESC) WHERE status='published';

ALTER TABLE comments ADD COLUMN site_id bigint;
UPDATE comments c SET site_id=a.site_id FROM articles a WHERE a.id=c.article_id;
ALTER TABLE comments ALTER COLUMN site_id SET NOT NULL,
 DROP CONSTRAINT comments_article_id_fkey, DROP CONSTRAINT comments_author_id_fkey,
 ADD FOREIGN KEY(article_id,site_id) REFERENCES articles(id,site_id) ON DELETE CASCADE,
 ADD FOREIGN KEY(author_id,site_id) REFERENCES users(id,site_id) ON DELETE CASCADE;

ALTER TABLE likes ADD COLUMN site_id bigint;
UPDATE likes l SET site_id=a.site_id FROM articles a WHERE a.id=l.article_id;
ALTER TABLE likes ALTER COLUMN site_id SET NOT NULL,
 DROP CONSTRAINT likes_article_id_fkey, DROP CONSTRAINT likes_user_id_fkey,
 ADD FOREIGN KEY(article_id,site_id) REFERENCES articles(id,site_id) ON DELETE CASCADE,
 ADD FOREIGN KEY(user_id,site_id) REFERENCES users(id,site_id) ON DELETE CASCADE;

-- The view expands a.* at creation time, so it is recreated to include articles.site_id.
DROP VIEW article_metrics;
CREATE VIEW article_metrics AS
 SELECT a.*,
 coalesce(v.views,0)::bigint AS views, coalesce(l.likes,0)::bigint AS likes, coalesce(c.comments,0)::bigint AS comments,
 (coalesce(v.views,0)+5*coalesce(l.likes,0)+3*coalesce(c.comments,0))::bigint AS popularity
 FROM articles a
 LEFT JOIN (SELECT article_id,sum(views)::bigint views FROM article_daily_views GROUP BY article_id) v ON v.article_id=a.id
 LEFT JOIN (SELECT article_id,count(*) likes FROM likes GROUP BY article_id) l ON l.article_id=a.id
 LEFT JOIN (SELECT article_id,count(*) comments FROM comments WHERE NOT deleted GROUP BY article_id) c ON c.article_id=a.id;
