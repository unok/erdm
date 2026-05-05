package layout

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Load は path の座標 JSON を読み込んで Layout として返す。
//
// 仕様（design.md §C9、要件 6.1 / 6.5 / 6.6）:
//   - ファイルが存在しない場合は空 Layout と nil を返す（LoadError にしない）。
//   - ファイルは存在するが読み取りに失敗した、もしくは JSON として破損している
//     場合は *LoadError を返し、HTTP サーバ側で 500 にマッピングできるよう
//     呼び出し側に区別可能な情報（Path / Cause）を提供する。
//   - 戻り値の Layout は常に非 nil（不存在時も空マップ）で、呼び出し側が
//     nil チェックなしで反復・参照できるよう保証する。
func Load(path string) (Layout, *LoadError) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Layout{}, nil
		}
		return nil, &LoadError{Path: path, Cause: err.Error()}
	}
	layout := Layout{}
	if err := json.Unmarshal(data, &layout); err != nil {
		return nil, &LoadError{Path: path, Cause: err.Error()}
	}
	return layout, nil
}

// Save は Layout を path に原子的に書き出す（要件 10.3）。
//
// 同一ディレクトリに `os.CreateTemp` で一時ファイルを作成し、書き込み完了後に
// `os.Rename` で原子的に置換する。POSIX の rename(2) は同一ファイルシステム上で
// 原子的なため、書き込み途中のクラッシュ等で元ファイルが破壊されることはない。
// 失敗時は一時ファイルを除去し、元ファイルは不変のまま残す。
func Save(path string, l Layout) error {
	if l == nil {
		l = Layout{}
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
