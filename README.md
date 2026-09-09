# AHC Plaza

AHC Plaza は [pahcer](https://github.com/terry-u16/pahcer) と連携して AtCoder Heuristic Contest（AHC）の C++ ソースファイル実行、結果保存、比較・分析をローカルでまとめて扱うためのブラウザで動作するGUIツールです。

詳しい使い方は[AHC Plazaの使い方](https://taigatappuri.net/blog/ahc-plaza/)をご参照ください。

## 主な機能

- 公式ジェネレータを使った入力ケース生成
- AtCoder公式ビジュアライザのローカル表示
- ソースファイル実行とソーススナップショット保存
- C++定数のOptunaチューニング（Python環境を同梱）
- ケースごとのスコア・実行時間・ログの確認
- 実行間の統計比較と入力条件による絞り込み

### 実行と履歴の確認

ソースファイルと入力セットを選んでテストを実行し、直近のスコア分布や実行状況を一覧で確認できます。

![実行設定と直近の実行履歴を表示した AHC Plaza の画面](./images/top_demo.png)

### 実行結果の比較

2つの実行結果について、平均スコア、差分、信頼区間、スコア分布を並べて比較できます。

![2つの実行結果のスコアと分布を比較した AHC Plaza の画面](./images/compare_demo.png)


## 対応環境

- Linux（amd64 / arm64）
- `g++`
- Rust / Cargo
- [pahcer](https://github.com/terry-u16/pahcer)

## Optunaによるチューニング

調整したいグローバル定数にコメントを付けます。

```cpp
constexpr int BEAM_WIDTH = 50;        // @tune 10 200
constexpr double START_TEMP = 100.0;  // @tune 1 1000 log
```

GUIの「チューニング」でソースと入力セットを選び、「パラメータを検出」から探索範囲を確認して開始します。現在値を評価した後、指定した回数だけ追加候補を試します。各候補はソースのコピーの初期値だけを変更して再コンパイルします。元ファイルは変更しません。

探索・検証用の入力セットは、`ahc-plaza/inputs/`直下のフォルダーから選択します（例：`ahc-plaza/inputs/train/0000.txt`）。通常Runの`default_input_dir`設定とは独立しています。追加したセットは「入力セットを再読込」で一覧へ反映できます。

- 目的値は固定ケースの**生スコア平均**です。最大化・最小化はプロジェクト設定に従います。
- WA・TLE・コンパイル失敗・結果の欠落は失敗Trialとし、最良値の計算に含めません。5回連続失敗で探索を停止します。
- 最良候補には現在値も含みます。「値を入れたC++を保存」で単独のC++を取得できます。
- 探索に使っていない入力セットで、現在値と候補を比較できます。検証はOptunaへ戻しません。
- 「候補の終了後に一時停止」と「今すぐ停止」を選べます。ブラウザを閉じてもサーバーが動いていれば継続します。
- 再開は保存した原本・入力・設定を使います。元ソースを編集しても探索に混入しません。探索範囲やソースを変える場合は新しいStudyを開始してください。
- TrialのRunは通常履歴から除外されます。「チューニングを含む」で表示できます。通常Runとチューニングの重い評価は同一プロジェクト内で直列に実行します。

対応するのは、単一C++ソースのグローバルな1行・1変数の数値リテラル初期化です。型は`int`・`long long`・`float`・`double`で、`static`・`const`・`constexpr`を付けられます。`step=100`による刻み指定、`log`による対数探索に対応します。型エイリアス・式による初期化・関数内・クラス内・条件付きプリプロセッサ内の注釈は診断します。pahcerのビルド設定はworkspaceの`main.cpp`を`g++`でコンパイルする必要があります。独自ヘッダーや複数翻訳単位は初版では対象外です。

### Python環境と容量

**Pythonの事前インストール・pip・uvの操作は不要です。** 配布バイナリに専用CPython 3.12.14、Optuna 5.0.0と必須依存を同梱しています。初回チューニング時にオフラインで自動展開し、システムPythonやPATHは変更しません。通常のRun・履歴・比較では展開しません。

同梱環境の目安は、amd64で圧縮約52 MB＋展開後約169 MB、arm64で圧縮約47 MB＋展開後約152 MBです。圧縮データは本体にも残ります。ソース、入力、tools、候補ごとのログ・出力の保存容量は別途増えます。実際の容量と保存場所はチューニング画面で確認できます。

専用環境はプロジェクト内の`ahc-plaza/runtime/<環境ハッシュ>/`へ保存します。対応するチューニング環境はglibcベースのLinux（glibc 2.28以上、amd64 / arm64）です。muslベースのLinuxは同梱環境の対象外です。通常機能の対応環境は従来どおりです。

GUIを終了してから、不要な専用環境を削除できます。旧バージョンの専用環境もまとめて削除します。探索履歴とRunは残り、同梱されている版は次の利用時に再展開できます。

```sh
ahc-plaza tune clean-runtime
```

Studyは`ahc-plaza/tuning/`、各評価Runは`ahc-plaza/runs/`へ保存します。実行中のファイルを手動で削除しないでください。同梱環境が更新されると既存Studyの再開を拒否する場合があります。その場合は保存時のPlazaを使用するか、新しいStudyを開始します。ソースの書き出しや保存済み結果の閲覧は引き続き利用できます。

### CLI

GUIと同じ処理をCLIからも利用できます。

```sh
ahc-plaza tune setup  # 任意: オフライン展開と起動確認
ahc-plaza doctor --tuning
ahc-plaza tune --solver solver/main.cpp --input-dir ahc-plaza/inputs/train --trials 100
ahc-plaza tune resume --study <study-id> --additional-trials 100
ahc-plaza tune export --study <study-id> --best
ahc-plaza tune validate --study <study-id> --input-dir ahc-plaza/inputs/validation --threads 1
```

GUIとCLIの同時所有はプロジェクトロックで防ぎます。CLIを使う際は同じプロジェクトのGUIを終了してください。時間予算は`--seconds`で指定でき、実行中の候補の終了まで待ちます。Optunaのseedはsolver内部の乱数やOS負荷による変動まで固定するものではありません。

## インストール

最新のGitHub Releaseからインストールします。

```sh
curl -fsSL https://github.com/taigatappuri/AHC-Plaza/releases/latest/download/install.sh | sh
```

標準のインストール先は `$HOME/.local/bin/ahc-plaza` です。変更する場合は `AHC_PLAZA_INSTALL_DIR` を指定します。

```sh
curl -fsSL https://github.com/taigatappuri/AHC-Plaza/releases/latest/download/install.sh \
  | AHC_PLAZA_INSTALL_DIR=/path/to/bin sh
```

インストール完了後、以下のコマンドでバージョンが適切に出力されることをご確認ください。
```
ahc-plaza --version
```

## 更新

現在実行している AHC Plaza を最新の GitHub Release に更新します。

```sh
ahc-plaza update
```

最新バージョンを確認し、更新がある場合だけ現在の実行ファイルと同じ場所へダウンロードします。

## アンインストール

標準のインストール先から AHC Plaza をアンインストールします。

```sh
ahc-plaza uninstall
```

インストール先を変更している場合は、`--install-dir`でそのディレクトリを指定します。

```sh
ahc-plaza uninstall --install-dir /path/to/bin
```

このコマンドで削除されるのは AHC Plaza の実行ファイルだけです。`ahc-plaza.toml`、`ahc-plaza/`ディレクトリ、保存済みの実行結果などのプロジェクトデータは削除されません。

## クイックスタート

AHC プロジェクトのルートで pahcer と AHC-Plaza を初期化します。


### pahcer の初期化
```sh
pahcer init --problem <PROBLEM_NAME> --objective <OBJECTIVE> --lang cpp
```
### AHC Plaza の初期化
```sh
ahc-plaza init --problem <PROBLEM_NAME> --objective <OBJECTIVE>
ahc-plaza doctor
```

`<OBJECTIVE>`には pahcer 同様、目標がスコアの最大化か最小化に合わせて指定してください。
- max: スコアが大きい方が良い
- min: スコアが小さい方が良い

初期化後のディレクトリ構成は次のようになります。
```text
ahc000/
├── tools/                  # 公式ローカルテスト環境
├── pahcer/                 # pahcer ディレクトリ
├── solver/
│   └── main.cpp
├── pahcer_config.toml      # pahcer の設定
├── ahc-plaza.toml          # AHC Plaza の設定
└── ahc-plaza/              # AHC Plaza の管理データ
    ├── inputs/             # AHC Plaza で生成した入力ケース
    ├── features/           # 派生特徴量の C++ ソース
    └── runs/               # 実行結果
```

`ahc-plaza init` は `solver/`、`ahc-plaza.toml`、`ahc-plaza/` 以下の各ディレクトリを作成します。`tools/` は、利用する問題のローカルテスト環境に合わせて用意してください。

GUIはローカルホストで起動します。

```sh
ahc-plaza gui --port 8080
```

ブラウザで`http://127.0.0.1:8080`を開いてください。

## ライセンス

AHC Plaza は[MIT License](./LICENSE)で公開しています。第三者著作物のライセンスは[THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md)を参照してください。

## 開発・配布ビルド

通常の開発チェックは`make check`です。資材を同梱しない`go build ./cmd/ahc-plaza`でも通常機能は動きますが、チューニングは未同梱と表示されます。

```sh
make web-install
make check
make build      # 現在のCPU向けPython環境を取得・同梱
make release    # amd64 / arm64をそれぞれ同梱
```

同梱ビルドには開発者側でPython 3.12以上とuvが必要です。ビルド時だけ、固定SHA-256のPython配布物と、ハッシュ付き`requirements.lock`のwheelを取得します。資材はGit管理外の`internal/tuning/runtime/assets/`に保存します。**利用者側ではこのビルド操作や環境構築は不要です。**

```sh
go test -tags tuning_bundle ./internal/tuning/...
python3 scripts/tuning/smoke.py --binary ./ahc-plaza --trials 100
```

後者にはg++とpahcerが必要です。一時プロジェクトでシステムPythonのないPATH、実Optuna、停止・強制終了・再開、固定条件、書き出し、別入力検証を確認します。amd64/arm64の同梱workerテストはCIにも定義しています。

容量の実測、工程別ベンチマーク、確認した環境と未確認の範囲は[実装の検証記録](docs/optuna-verification.md)を参照してください。

Pythonと依存の更新時は`scripts/tuning/python-lock.json`・`requirements.lock`を更新し、両CPUの同梱テスト、通常機能の回帰テスト、容量とライセンスの確認を行います。版固定はセキュリティ更新を止める方針ではありません。同梱物の一覧と第三者ライセンスについては[THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md)を参照してください。
