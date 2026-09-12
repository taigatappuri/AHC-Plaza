# Optuna実装の検証記録

2026-09-10、Linux amd64、AMD Ryzen 7 7700、Go 1.27.0で確認した記録です。

## 配布・利用条件

Python 3.12.14、Optuna 5.0.0と必須依存をCPU別に同梱しています。利用者がPython・pip・uvをインストールする操作や、初回展開時のダウンロードはありません。g++・pahcer・問題固有のツールは従来どおり必要です。

| CPU | 配布バイナリ全体 | 内蔵圧縮データ | 専用環境の展開サイズ |
| --- | ---: | ---: | ---: |
| amd64 | 約71.3 MB | 52,362,508 bytes | 168,885,099 bytes |
| arm64 | 約64.5 MB | 46,696,005 bytes | 151,569,023 bytes |

MBは10進表記です。展開環境はプロジェクトごとに保存します。Studyの固定ソース・入力・toolsと、候補ごとのworkspace・ログ・出力・DBはこの容量に含みません。

配布元・版・SHA-256・wheelハッシュを固定し、同梱物のライセンス文とパッケージング変更内容を保持しています。元ソースは上書きせず、探索中の編集も固定条件に取り込みません。更新手順・削除手順・対応範囲はREADMEに記載しています。

## 実施した確認

- `make check`: 全Goテスト、go vet、Svelteチェック（エラー・警告とも0）。
- `npm --prefix web test`: 既存の7テストが成功。
- `go test -race -tags tuning_bundle ./internal/tuning/... ./internal/store ./internal/server ./internal/process`: 実workerを含むrace検査。
- `make release`: amd64・arm64の同梱バイナリとチェックサムを生成。
- `python3 scripts/tuning/smoke.py --binary ./ahc-plaza --trials 100`: 実C++・pahcer・Optunaで2パラメータを探索。通常Run、最良C++の書き出しと再評価、元ソース・入力の変更、追加試行、停止・SIGKILL後の復旧を確認。計112候補中110成功、意図的中断2件。未配信outboxとOptunaの未完了Trialが残らないことを確認。
- 同じスクリプトで最小化、全候補のビルド失敗、5連続失敗時の停止、現在値が最良の場合、固定資材の改変拒否、専用環境だけの削除を確認。
- PythonのないPATHで未展開の通常Run・doctorと探索を確認。別途、Python未導入のUbuntu 24.04コンテナを`--network none`で起動し、同梱環境の展開とNumPy・SQLAlchemy・Optunaのimportを確認。
- Chromiumで検出、プロファイル保存、探索開始・完了、C++ダウンロードを確認。最終バイナリでも2パラメータ・2試行とダウンロードを確認。
- パーサーのCRLF・文字列・コメント・行継続・定数式のコンパイル、整数精度・float丸め、結果の欠落・重複seed・WA/TLE・負値・ゼロ、別プロセスグループの子孫停止を個別に検証。

強制終了テスト中に外部DB読取りとの短い競合で`SQLITE_BUSY`が発生したため、全接続に5秒のbusy timeoutと書込みトランザクションのIMMEDIATE開始を設定しました。外部読取りロックの解放後に保存が完了する回帰テストを追加し、100試行テストを再実行しています。接続パラメータは使用版のソースと[modernc.org/sqliteの仕様](https://pkg.go.dev/modernc.org/sqlite#Driver.Open)を確認しています。

## 工程別の測定

次のベンチマークは小型・大型の**合成solver**を使います。実問題の探索時間を予測するものではありません。

```sh
go test ./internal/tuning -run '^$' -bench BenchmarkTuningPhases -benchtime=1x -count=3
```

3回の中央値です。ケース実行は直列のプロセス起動を含み、pahcer・Optunaの管理時間は含みません。DBはRun作成とケース結果の確定を測定しています。

| 工程 | 小型（2ケース、tools 1 KiB） | 大型（1,500関数、100ケース、tools 16 MiB） |
| --- | ---: | ---: |
| workspace準備 | 2.4 ms | 21.4 ms |
| g++再コンパイル | 80.0 ms | 1,835.8 ms |
| 全ケースの実行 | 1.5 ms | 76.0 ms |
| DB保存 | 15.6 ms | 15.3 ms |
| workspaceへコピーするソース・tools・入力 | 1,268 bytes | 16,951,318 bytes |

この大型サンプルではコンパイル時間が支配的です。また100候補でコピーだけでも約1.7 GBとなるため、Python環境のサイズだけで必要容量を見積もらないでください。画面にはStudyと関連Runの使用容量、ビルドを含む実測経過時間からの残り時間目安を表示します。

## 確認範囲の限界

arm64はローカルでクロスビルドまで確認しました。ネイティブworkerテストをCIに定義していますが、この作業ではリモートCIを実行していません。対象Linuxはglibc環境です。

性能測定は合成solverによるもので、実際の大型AHC solver・公式テスターを網羅していません。ソースの規模やtoolsの容量、ディスク、問題の実行時間で費用は変わります。Optunaが必ず最適解を得るという保証は検証条件に含めていません。
