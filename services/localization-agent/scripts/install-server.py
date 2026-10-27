#!/usr/bin/env python3
"""Root-only initial production installation. Never prints connection strings or tokens.
Run after reviewing sql/001_up.sql, sql/002_role.sql and production approval notes.
Existing schemas, roles or secret files are deliberately refused; upgrades are separate.
"""
import base64
from datetime import datetime, timezone
import hashlib
import hmac
import json
import os
import re
from pathlib import Path
import secrets
import shlex
import subprocess
import sys
from urllib.parse import unquote, urlsplit

ROOT=Path(__file__).resolve().parents[1]
SECRETS=Path('/opt/platform/secrets')


def run(command, label, data=None, output=None):
    result=subprocess.run(command,input=data,stdout=output if output else subprocess.PIPE,stderr=subprocess.PIPE)
    if result.returncode:
        state=re.search(rb'(?:ERROR|FATAL):\s+([A-Z0-9]{5})\b',result.stderr)
        raise RuntimeError(label+'_failed'+('_'+state.group(1).decode() if state else ''))
    return result.stdout


def scram(password):
    salt=secrets.token_bytes(16)
    salted=hashlib.pbkdf2_hmac('sha256',password.encode(),salt,4096)
    stored=hashlib.sha256(hmac.new(salted,b'Client Key','sha256').digest()).digest()
    server=hmac.new(salted,b'Server Key','sha256').digest()
    b64=lambda value:base64.b64encode(value).decode()
    return f'SCRAM-SHA-256$4096:{b64(salt)}${b64(stored)}:{b64(server)}'


def main():
    if os.geteuid()!=0: raise RuntimeError('root_required')
    os.umask(0o077)
    secret_paths=[SECRETS/'localization.env',SECRETS/'localization-control.env']
    if any(p.exists() for p in secret_paths): raise RuntimeError('existing_secret_refuse_overwrite')
    source=run(['bash','-c','set -a; . "$1"; printf "%s" "$DATABASE_URL"','localization-installer',str(SECRETS/'leksuu.env')],'read_database_name').decode()
    database=unquote(urlsplit(source).path.lstrip('/'))
    del source
    if not database or '/' in database: raise RuntimeError('database_name_invalid')
    base=['docker','exec','-i','-e','TARGET_DATABASE='+database,'platform-postgres','sh','-c']
    psql=base+['exec psql -X -q -v ON_ERROR_STOP=1 -v VERBOSITY=sqlstate -U "$POSTGRES_USER" -d "$TARGET_DATABASE" -At']
    preflight=run(psql,'preflight',b"SELECT (EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='localization') OR EXISTS(SELECT 1 FROM pg_roles WHERE rolname='localization_service'))::int;").decode().strip()
    if preflight!='0': raise RuntimeError('existing_schema_or_role_refuse_overwrite')
    stamp=datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')
    backup=Path('/root')/('localization-install-'+stamp)
    backup.mkdir(mode=0o700)
    with (backup/'leksuu-before.dump').open('wb') as out:
        run(base+['exec pg_dump -Fc -U "$POSTGRES_USER" -d "$TARGET_DATABASE"'],'database_backup',output=out)
    run(['docker','exec','-i','platform-postgres','pg_restore','--list'],'backup_validation',(backup/'leksuu-before.dump').read_bytes())
    password=secrets.token_urlsafe(36)
    token=secrets.token_urlsafe(48)
    db_identifier='"'+database.replace('"','""')+'"'
    db_uri='postgresql://localization_service:'+password+'@platform-postgres:5432/'+database
    settings={'LOCALIZATION_DATABASE_URL':db_uri,'MCP_BEARER_TOKEN':token,'MCP_ALLOWED_HOST':'leksuu.ir','MCP_PUBLIC_URL':'https://leksuu.ir/localization/mcp'}
    prepared=[]
    for path in secret_paths:
        values=dict(settings)
        if path.name=='localization-control.env':
            values.update(OPENAI_API_KEY='',OPENAI_MODEL='',APPROVED_BUDGET_USD='5')
        temporary=path.with_name(path.name+'.pending-'+stamp)
        fd=os.open(temporary,os.O_CREAT|os.O_EXCL|os.O_WRONLY,0o600)
        with os.fdopen(fd,'w') as f:
            f.write('\n'.join(k+'='+shlex.quote(v) for k,v in values.items())+'\n')
        prepared.append((temporary,path))
    up=(ROOT/'sql/001_up.sql').read_text().rsplit('COMMIT;',1)[0]
    # Prevent the password verifier and other sensitive statements from entering SQL logs.
    up=up.replace('BEGIN;',"BEGIN;\nSET LOCAL log_statement='none';\nSET LOCAL log_min_duration_statement=-1;\nSET LOCAL log_duration=off;\nSET LOCAL log_min_error_statement='panic';",1)
    role=(ROOT/'sql/002_role.sql').read_text()
    audit="""
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['courses','units','lessons','lesson_items','exercises'] LOOP
  IF has_table_privilege('localization_service','public.'||t,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER') THEN
   RAISE EXCEPTION 'unsafe_source_privileges';
  END IF;
 END LOOP;
 IF has_schema_privilege('localization_service','public','CREATE') OR
    has_schema_privilege('localization_service','localization','CREATE') OR
    has_column_privilege('localization_service','localization.drafts','status','UPDATE') THEN
  RAISE EXCEPTION 'unsafe_schema_or_approval_privileges';
 END IF;
 IF EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
   WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m','f')
   AND c.relname NOT IN ('courses','units','lessons','lesson_items','exercises')
   AND has_table_privilege('localization_service',c.oid,'SELECT')) THEN
  RAISE EXCEPTION 'unrelated_table_read_access';
 END IF;
END $$;
"""
    sql=up+role+'\nGRANT CONNECT ON DATABASE '+db_identifier+' TO localization_service;\n'
    sql+="ALTER ROLE localization_service PASSWORD '"+scram(password)+"';\n"+audit+'\nCOMMIT;\n'
    try:
        run(psql,'transactional_migration',sql.encode())
    except Exception:
        for temporary,_ in prepared: temporary.unlink(missing_ok=True)
        raise
    for temporary,path in prepared: temporary.replace(path)
    receipt={'installed_at_utc':stamp,'database_backup':str(backup/'leksuu-before.dump'),'schema':'localization','role':'localization_service','secret_files':[str(p) for p in secret_paths],'paid_requests':0}
    (backup/'receipt.json').write_text(json.dumps(receipt,indent=2)+'\n')
    print(json.dumps(receipt,indent=2))

if __name__=='__main__':
    try: main()
    except Exception as error:
        code=str(error) if isinstance(error,RuntimeError) else type(error).__name__
        print(json.dumps({'status':'failed','error_code':code}))
        sys.exit(1)
