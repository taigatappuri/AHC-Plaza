#!/bin/sh
set -eu

if [ "$(uname -s)" != "Linux" ]; then
  echo "AHC PlazaはLinuxのみ対応しています。" >&2
  exit 1
fi

machine=$(uname -m)
case "$machine" in
  x86_64|amd64) asset="ahc-plaza-linux-amd64" ;;
  aarch64|arm64) asset="ahc-plaza-linux-arm64" ;;
  *) echo "未対応のCPUアーキテクチャです: $machine" >&2; exit 1 ;;
esac

install_dir=${AHC_PLAZA_INSTALL_DIR:-"$HOME/.local/bin"}
version=${AHC_PLAZA_VERSION:-latest}
base_url=${AHC_PLAZA_RELEASE_BASE_URL:-"https://github.com/taigatappuri/AHC-Plaza"}
if [ "$version" = "latest" ]; then
  download_url="$base_url/releases/latest/download"
else
  download_url="$base_url/releases/download/$version"
fi

temp_dir=$(mktemp -d)
cleanup() { rm -rf "$temp_dir"; }
trap cleanup EXIT INT TERM

echo "AHC Plazaをダウンロードしています: $asset"
curl --fail --silent --show-error --location \
  "$download_url/$asset" -o "$temp_dir/$asset"
curl --fail --silent --show-error --location \
  "$download_url/checksums.txt" -o "$temp_dir/checksums.txt"

expected=$(awk -v name="$asset" '$2 == name || $2 == "./" name {print $1; exit}' "$temp_dir/checksums.txt")
if [ -z "$expected" ]; then
  echo "チェックサムが見つかりません: $asset" >&2
  exit 1
fi
actual=$(sha256sum "$temp_dir/$asset" | awk '{print $1}')
if [ "$expected" != "$actual" ]; then
  echo "チェックサム検証に失敗しました。" >&2
  exit 1
fi

mkdir -p "$install_dir"
chmod 0755 "$temp_dir/$asset"
mv "$temp_dir/$asset" "$install_dir/ahc-plaza"
echo "インストールしました: $install_dir/ahc-plaza"
if ! command -v ahc-plaza >/dev/null 2>&1; then
  echo "PATHに $install_dir を追加してください。"
fi
"$install_dir/ahc-plaza" --version
