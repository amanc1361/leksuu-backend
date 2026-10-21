#!/usr/bin/env python3
"""Disposable localhost-only PostgreSQL, actual SQL migrations and least-privilege tests."""
import os
from pathlib import Path
import secrets
import subprocess
import sys
import time

root=Path(__file__).resolve().parents[1]
name='localization-test-'+secrets.token_hex(4)
env={**os.environ,'POSTGRES_PASSWORD':secrets.token_urlsafe(24)}
try:
    subprocess.run(['docker','run','-d','--rm','--name',name,'-e','POSTGRES_PASSWORD','-e','POSTGRES_DB=localization_test','-p','127.0.0.1::5432','postgres:17-alpine'],env=env,check=True,stdout=subprocess.DEVNULL)
    for _ in range(40):
        if subprocess.run(['docker','exec',name,'pg_isready','-U','postgres'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0: break
        time.sleep(.25)
    port=subprocess.check_output(['docker','port',name,'5432'],text=True).strip().rsplit(':',1)[1]
    env['LOCALIZATION_TEST_DATABASE_URL']=f"postgresql://postgres:{env['POSTGRES_PASSWORD']}@127.0.0.1:{port}/localization_test"
    sys.exit(subprocess.run([str(root/'.venv/bin/pytest'),'tests','-q'],cwd=root,env=env).returncode)
finally:
    subprocess.run(['docker','rm','-f',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
