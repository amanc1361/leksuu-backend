import json
import os
from pathlib import Path
import pytest

@pytest.fixture
def context():
    return json.loads((Path(__file__).parent/'fixtures/real-lesson.json').read_text())

@pytest.fixture
def store(context):
    from localizer.store import Store
    from psycopg import sql
    from psycopg.types.json import Jsonb
    import psycopg
    dsn=os.environ.get('LOCALIZATION_TEST_DATABASE_URL')
    if not dsn:
        pytest.skip('LOCALIZATION_TEST_DATABASE_URL required for real PostgreSQL integration')
    # This fixture is deliberately destructive ONLY to a dedicated test database name.
    with psycopg.connect(dsn) as c:
        if not c.info.dbname.startswith('localization_test'):
            raise RuntimeError('dedicated_test_database_required')
        c.execute('DROP SCHEMA IF EXISTS localization CASCADE')
        c.execute('DROP SCHEMA public CASCADE; CREATE SCHEMA public')
        c.execute('CREATE TABLE courses(id uuid PRIMARY KEY,language text,cefr_level text,title text,description text)')
        c.execute('CREATE TABLE units(id uuid PRIMARY KEY,course_id uuid REFERENCES courses(id),title text)')
        c.execute('CREATE TABLE lessons(id uuid PRIMARY KEY,course_id uuid REFERENCES courses(id),unit_id uuid REFERENCES units(id),title text,objective jsonb)')
        c.execute('CREATE TABLE lesson_items(id uuid PRIMARY KEY,lesson_id uuid REFERENCES lessons(id),position int,kind text,payload jsonb)')
        c.execute('CREATE TABLE exercises(id uuid PRIMARY KEY,lesson_item_id uuid REFERENCES lesson_items(id),type text,prompt jsonb,answer_key jsonb)')
        for table, rows in [('courses',[context['course']]),('units',[context['unit']]),('lessons',[context['lesson']]),('lesson_items',context['items']),('exercises',context['exercises'])]:
            columns=[r[0] for r in c.execute('SELECT column_name FROM information_schema.columns WHERE table_schema=%s AND table_name=%s ORDER BY ordinal_position',('public',table)).fetchall()]
            for row in rows:
                values=[Jsonb(row.get(k)) if isinstance(row.get(k),(dict,list)) else row.get(k) for k in columns]
                c.execute(sql.SQL('INSERT INTO {} ({}) VALUES ({})').format(sql.Identifier(table),sql.SQL(',').join(map(sql.Identifier,columns)),sql.SQL(',').join(sql.Placeholder() for _ in columns)),values)
        c.execute((Path(__file__).parents[1]/'sql/001_up.sql').read_text())
    return Store(dsn)
