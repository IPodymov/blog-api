-- Publishing is limited to authors; self-registered users stay 'user'. Nobody is promoted automatically.
ALTER TABLE users DROP CONSTRAINT users_role_check,
 ADD CONSTRAINT users_role_check CHECK(role IN ('user','author','admin'));
