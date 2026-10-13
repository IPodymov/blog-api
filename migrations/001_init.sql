CREATE TABLE users (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 username text NOT NULL UNIQUE CHECK (username ~ '^[a-z0-9_]{3,32}$'),
 email text NOT NULL UNIQUE CHECK (email=lower(email) AND length(email) BETWEEN 3 AND 254), password_hash text NOT NULL,
 display_name text NOT NULL DEFAULT '' CHECK(length(display_name)<=100),
 avatar_url text NOT NULL DEFAULT '' CHECK(length(avatar_url)<=2048), bio text NOT NULL DEFAULT '' CHECK(length(bio)<=2000),
 role text NOT NULL DEFAULT 'user' CHECK(role IN ('user','admin')), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE articles (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, author_id bigint NOT NULL REFERENCES users ON DELETE CASCADE,
 title text NOT NULL CHECK(length(title) BETWEEN 1 AND 200), slug text NOT NULL UNIQUE CHECK(slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(slug)<=200),
 description text NOT NULL DEFAULT '' CHECK(length(description)<=500), content text NOT NULL CHECK(length(content) BETWEEN 1 AND 100000),
 status text NOT NULL CHECK(status IN ('draft','published')), version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), published_at timestamptz,
 CHECK ((status='published')=(published_at IS NOT NULL))
);
CREATE INDEX articles_author_status ON articles(author_id,status,id DESC);
CREATE INDEX articles_publication ON articles(published_at DESC,id DESC) WHERE status='published';
CREATE INDEX articles_search ON articles USING gin(to_tsvector('simple',title || ' ' || description || ' ' || content));
CREATE TABLE tags(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, name text NOT NULL UNIQUE CHECK(name ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(name)<=40));
CREATE TABLE article_tags(article_id bigint REFERENCES articles ON DELETE CASCADE, tag_id bigint REFERENCES tags ON DELETE CASCADE, PRIMARY KEY(article_id,tag_id));
CREATE INDEX article_tags_tag ON article_tags(tag_id,article_id);
CREATE TABLE comments (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, article_id bigint NOT NULL REFERENCES articles ON DELETE CASCADE,
 author_id bigint NOT NULL REFERENCES users ON DELETE CASCADE, parent_id bigint, depth int NOT NULL DEFAULT 0 CHECK(depth BETWEEN 0 AND 1),
 content text NOT NULL CHECK(length(content)<=5000), deleted boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(id,article_id), FOREIGN KEY(parent_id,article_id) REFERENCES comments(id,article_id) ON DELETE CASCADE,
 CHECK((parent_id IS NULL AND depth=0) OR (parent_id IS NOT NULL AND depth=1)), CHECK((deleted AND content='') OR (NOT deleted AND length(content)>0))
);
CREATE INDEX comments_article ON comments(article_id,id);
CREATE FUNCTION check_comment_parent() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.parent_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM comments WHERE id=NEW.parent_id AND article_id=NEW.article_id AND depth=0 AND NOT deleted) THEN
 RAISE EXCEPTION 'invalid comment parent' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER comment_parent BEFORE INSERT ON comments FOR EACH ROW EXECUTE FUNCTION check_comment_parent();
CREATE TABLE likes(article_id bigint REFERENCES articles ON DELETE CASCADE,user_id bigint REFERENCES users ON DELETE CASCADE,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(article_id,user_id));
CREATE TABLE session_families(id text PRIMARY KEY,user_id bigint NOT NULL REFERENCES users ON DELETE CASCADE, expires_at timestamptz NOT NULL,revoked boolean NOT NULL DEFAULT false);
CREATE INDEX session_families_user ON session_families(user_id);
CREATE TABLE refresh_sessions(token_hash text PRIMARY KEY,family_id text NOT NULL REFERENCES session_families ON DELETE CASCADE,used boolean NOT NULL DEFAULT false,created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX refresh_sessions_family ON refresh_sessions(family_id);
CREATE TABLE access_sessions(token_hash text PRIMARY KEY,family_id text NOT NULL REFERENCES session_families ON DELETE CASCADE,expires_at timestamptz NOT NULL);
CREATE INDEX access_sessions_family ON access_sessions(family_id);
CREATE TABLE view_dedup(article_id bigint REFERENCES articles ON DELETE CASCADE, visitor_hash text NOT NULL,last_seen timestamptz NOT NULL, PRIMARY KEY(article_id,visitor_hash));
CREATE INDEX view_dedup_expiry ON view_dedup(last_seen);
CREATE TABLE article_daily_views(article_id bigint REFERENCES articles ON DELETE CASCADE,day date NOT NULL,views bigint NOT NULL CHECK(views>=0),PRIMARY KEY(article_id,day));
CREATE INDEX daily_views_day ON article_daily_views(day,article_id);
CREATE VIEW article_metrics AS
 SELECT a.*,
 coalesce(v.views,0)::bigint AS views, coalesce(l.likes,0)::bigint AS likes, coalesce(c.comments,0)::bigint AS comments,
 (coalesce(v.views,0)+5*coalesce(l.likes,0)+3*coalesce(c.comments,0))::bigint AS popularity
 FROM articles a
 LEFT JOIN (SELECT article_id,sum(views)::bigint views FROM article_daily_views GROUP BY article_id) v ON v.article_id=a.id
 LEFT JOIN (SELECT article_id,count(*) likes FROM likes GROUP BY article_id) l ON l.article_id=a.id
 LEFT JOIN (SELECT article_id,count(*) comments FROM comments WHERE NOT deleted GROUP BY article_id) c ON c.article_id=a.id;
