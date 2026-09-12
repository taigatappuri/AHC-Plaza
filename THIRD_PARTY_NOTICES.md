# Third-Party Notices

AHC Plazaが利用する第三者製ソフトウェアには、それぞれのライセンスが適用されます。

## IBM Plex Sans JP / IBM Plex Mono

- Copyright 2017–2018 IBM Corp. All rights reserved.
- License: SIL Open Font License 1.1
- Source: https://github.com/IBM/plex
- Web package: `@fontsource/ibm-plex-sans-jp` 5.3.0 / `@fontsource/ibm-plex-mono` 5.3.0
- License text: `licenses/IBM-Plex-OFL-1.1.txt`

## チューニング用Python・Optuna

正式配布には次の専用環境を同梱しています。システムPythonとは独立して動作します。

- CPython 3.12.14 — Python Software Foundation License等
- 配布元: https://github.com/astral-sh/python-build-standalone/releases/tag/20260901
- Optuna 5.0.0 — MIT License、Copyright Preferred Networks, Inc.
- Optuna source: https://github.com/optuna/optuna
- 必須依存: NumPy、SQLAlchemy、Alembic、colorlog、packaging、tqdm、PyYAML、greenlet、Mako、MarkupSafe、typing_extensions。採用版・ハッシュは`scripts/tuning/requirements.lock`に記録しています。

展開した環境の`PLAZA-NOTICES.txt`にライセンスファイル一覧、`licenses/`にPython組み込みライブラリのライセンス文を収録しています。各wheelに付属する著作権表示・ライセンス文も`lib/python3.12/site-packages/`の`.dist-info`等に保持しています。NumPy wheelに同梱されたOpenBLAS・Fortranランタイム等の通知も含まれます。必要なライセンス文は配布バイナリ内の圧縮資材にも同梱されています。

Pythonソース自体は変更していません。パッケージング時に不要な`_dbm`（Berkeley DB）、pip、バイトコードキャッシュ、重複するPython実行ファイルへのリンクを除外し、固定版のOptuna依存を追加しています。変更概要は専用環境の`PLAZA-CHANGES.txt`にも保存しています。

ビルド専用のuvおよびzstandardは利用者向けバイナリには同梱しません。
