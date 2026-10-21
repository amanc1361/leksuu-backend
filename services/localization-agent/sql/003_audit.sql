-- Read-only DBA checks after creating the service role.
SELECT rolname,rolsuper,rolcreatedb,rolcreaterole,rolreplication FROM pg_roles WHERE rolname='localization_service';
SELECT table_name,
 has_table_privilege('localization_service','public.'||table_name,'SELECT') AS can_read,
 has_table_privilege('localization_service','public.'||table_name,'UPDATE') AS must_be_false_update,
 has_table_privilege('localization_service','public.'||table_name,'INSERT') AS must_be_false_insert,
 has_table_privilege('localization_service','public.'||table_name,'DELETE') AS must_be_false_delete
FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('courses','units','lessons','lesson_items','exercises');
SELECT has_schema_privilege('localization_service','public','CREATE') AS must_be_false_create,
 has_column_privilege('localization_service','localization.drafts','status','UPDATE') AS must_be_false_approve;
SELECT r.rolname AS inherited_role FROM pg_auth_members m JOIN pg_roles r ON r.oid=m.roleid JOIN pg_roles u ON u.oid=m.member WHERE u.rolname='localization_service';
