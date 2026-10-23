#!/usr/bin/env python3
"""Initial workspace install. Root-only; backs up DB/env and never prints secrets."""
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shlex
import subprocess
from datetime import datetime,timezone
from urllib.parse import urlsplit,unquote

ROOT=Path(__file__).resolve().parents[1]


def command(args,data=None,output=None):
    result=subprocess.run(args,input=data,stdout=output or subprocess.PIPE,stderr=subprocess.PIPE)
    if result.returncode:
        raise RuntimeError('operator_command_failed')
    return result.stdout


def main():
    if os.geteuid()!=0:
        raise RuntimeError('root_required')
    os.umask(0o077)
    env=Path('/opt/platform/secrets/leksuu.env')
    admin_env=env.with_name('localization-admin.env')
    if admin_env.exists():
        raise RuntimeError('existing_install_use_upgrade_procedure')
    dsn=command(['bash','-c','set -a; . "$1"; printf "%s" "$DATABASE_URL"','workspace-install',str(env)]).decode()
    name=unquote(urlsplit(dsn).path.lstrip('/'))
    if not name or '/' in name:
        raise RuntimeError('invalid_database_name')
    base=['docker','exec','-i','-e','TARGET_DATABASE='+name,'platform-postgres','sh','-c']
    psql=base+['exec psql -X -q -v ON_ERROR_STOP=1 -v VERBOSITY=sqlstate -U "$POSTGRES_USER" -d "$TARGET_DATABASE" -At']
    if command(psql,b"SELECT (EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='localization_admin') OR EXISTS(SELECT 1 FROM pg_roles WHERE rolname='localization_admin_service'))::int").strip()!=b'0':
        raise RuntimeError('workspace_already_exists')
    backup=Path('/root')/('localization-admin-install-'+datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ'))
    backup.mkdir(mode=0o700)
    (backup/'leksuu.env.before').write_bytes(env.read_bytes())
    with (backup/'database-before.dump').open('wb') as output:
        command(base+['exec pg_dump -Fc -U "$POSTGRES_USER" -d "$TARGET_DATABASE"'],output=output)
    command(['docker','exec','-i','platform-postgres','pg_restore','--list'],(backup/'database-before.dump').read_bytes())
    for container in ('leksuu-api-1','leksuu-frontend-1'):
        value=command(['docker','inspect',container,'--format','{{.Image}}']).decode().strip()
        (backup/(container+'.image')).write_text(value+'\n')
    spec=importlib.util.spec_from_file_location('initial_installer',ROOT/'scripts/install-server.py')
    helper=importlib.util.module_from_spec(spec); spec.loader.exec_module(helper)
    password=secrets.token_urlsafe(36); token=secrets.token_urlsafe(48)
    migration=(ROOT/'sql/004_admin_workspace.sql').read_text().replace('BEGIN;',"BEGIN;\nSET LOCAL log_statement='none';\nSET LOCAL log_min_duration_statement=-1;\nSET LOCAL log_duration=off;\nSET LOCAL log_min_error_statement='panic';",1).rsplit('COMMIT;',1)[0]
    migration+=(ROOT/'sql/006_translation_memory.sql').read_text().replace('BEGIN;','').replace('COMMIT;','')
    migration+='''
CREATE ROLE localization_admin_service LOGIN;
GRANT USAGE ON SCHEMA public,localization,localization_admin TO localization_admin_service;
GRANT SELECT ON public.courses,public.units,public.lessons,public.lesson_items,public.exercises,public.content_translations TO localization_admin_service;
GRANT UPDATE(objective) ON public.lessons TO localization_admin_service;
GRANT UPDATE(prompt) ON public.exercises TO localization_admin_service;
GRANT UPDATE(payload) ON public.lesson_items TO localization_admin_service;
GRANT INSERT ON public.content_translations TO localization_admin_service;
GRANT UPDATE(text,origin) ON public.content_translations TO localization_admin_service;
GRANT SELECT,INSERT,UPDATE ON ALL TABLES IN SCHEMA localization_admin TO localization_admin_service;
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA localization_admin TO localization_admin_service;
GRANT EXECUTE ON FUNCTION localization.lock_source() TO localization_admin_service;
'''
    migration+='ALTER ROLE localization_admin_service PASSWORD '+"'"+helper.scram(password)+"';\n"
    migration+='GRANT CONNECT ON DATABASE "'+name.replace('"','""')+'" TO localization_admin_service;\nCOMMIT;\n'
    command(psql,migration.encode())
    service_dsn='postgresql://localization_admin_service:'+password+'@platform-postgres:5432/'+name
    settings={'LOCALIZATION_ADMIN_DATABASE_URL':service_dsn,'LOCALIZATION_ADMIN_TOKEN':token,'LOCALIZATION_PROVIDER_DIR':'/state/providers'}
    admin_env.write_text(''.join(k+'='+shlex.quote(v)+'\n' for k,v in settings.items()))
    original=env.read_text()
    if not original.endswith('\n'): original+='\n'
    env.write_text(original+'LOCALIZATION_ADMIN_URL=http://leksuu-localization-admin-workspace-1:8000\nLOCALIZATION_ADMIN_TOKEN='+shlex.quote(token)+'\n')
    receipt={'status':'installed','backup_path':str(backup),'schema':'localization_admin','role':'localization_admin_service','source_changes':0,'provider_requests':0}
    (backup/'receipt.json').write_text(json.dumps(receipt,indent=2)+'\n')
    print(json.dumps(receipt))


if __name__=='__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'status':'error','error_code':str(error) if isinstance(error,RuntimeError) else type(error).__name__}))
        raise SystemExit(1)
