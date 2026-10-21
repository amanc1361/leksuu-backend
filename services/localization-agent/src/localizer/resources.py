import json
import os
from pathlib import Path
from .content import digest

ROOT=Path(os.environ.get('LOCALIZATION_ROOT',Path(__file__).resolve().parents[2]))


def rules():
    return {'instructions':(ROOT/'config/instructions-v1.md').read_text(),
            'glossary':json.loads((ROOT/'config/glossary-v1.json').read_text())}


def versions():
    data=rules()
    return ('educational-v1@'+digest(data['instructions']),str(data['glossary']['version'])+'@'+digest(data['glossary']))
