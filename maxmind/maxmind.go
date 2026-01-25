package maxmind

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	cfg "github.com/ibp-network/ibp-geodns-libs/config"
	log "github.com/ibp-network/ibp-geodns-libs/logging"
)

func updateMaxmindDatabase() error {
	c := cfg.GetConfig()
	baseDir := filepath.Join(c.Local.Maxmind.MaxmindDBPath)

	accountID := c.Local.Maxmind.AccountID
	licenseKey := c.Local.Maxmind.LicenseKey
	if accountID == "" || licenseKey == "" {
		// If credentials are missing but local DBs already exist, allow startup to continue.
		if haveLocalMaxmindDatabases(baseDir) {
			log.Log(log.Warn, "MaxMind credentials missing; using existing local databases only")
			return nil
		}
		return fmt.Errorf("maxmind AccountID or LicenseKey is missing; cannot download databases and no local copy found")
	}

	downloads := []struct {
		name         string
		editionID    string
		filenameLite string
		markerFile   string
	}{
		{"CityLite", "GeoLite2-City", "CityLite.mmdb", ".CityLite"},
		{"CountryLite", "GeoLite2-Country", "CountryLite.mmdb", ".CountryLite"},
		{"AsnLite", "GeoLite2-ASN", "AsnLite.mmdb", ".AsnLite"},
	}

	for _, dl := range downloads {
		if err := checkAndDownloadOne(baseDir, accountID, licenseKey, dl.name, dl.editionID, dl.filenameLite, dl.markerFile); err != nil {
			// If the specific DB is missing locally, this is fatal. Otherwise continue.
			localPath := filepath.Join(baseDir, dl.filenameLite)
			if st, statErr := os.Stat(localPath); statErr != nil || st.IsDir() {
				return err
			}
			log.Log(log.Warn, "Proceeding with existing %s due to download error: %v", dl.name, err)
		}
	}

	return nil
}

func haveLocalMaxmindDatabases(baseDir string) bool {
	required := []string{
		filepath.Join(baseDir, "CityLite.mmdb"),
		filepath.Join(baseDir, "CountryLite.mmdb"),
		filepath.Join(baseDir, "AsnLite.mmdb"),
	}
	for _, path := range required {
		if st, err := os.Stat(path); err != nil || st.IsDir() {
			return false
		}
	}
	return true
}

func checkAndDownloadOne(
	baseDir, accountID, licenseKey, dbName, editionID, mmdbFilename, markerFilename string,
) error {
	localMmdbPath := filepath.Join(baseDir, mmdbFilename)
	localMarkerPath := filepath.Join(baseDir, markerFilename)

	remoteURL := fmt.Sprintf(
		"https://download.maxmind.com/geoip/databases/%s/download?edition_id=%s&suffix=tar.gz",
		editionID, editionID,
	)

	remoteModTime, err := getRemoteLastModified(remoteURL, accountID, licenseKey)
	if err != nil {
		if st, statErr := os.Stat(localMmdbPath); statErr == nil && !st.IsDir() {
			log.Log(log.Warn, "%s HEAD request failed, using existing local db: %v", dbName, err)
			return nil
		}
		return fmt.Errorf("%s HEAD request error: %w", dbName, err)
	}
	if remoteModTime == "" {
		log.Log(log.Warn, "No Last-Modified header for %s from server. Will always download it.", dbName)
		remoteModTime = "no-last-mod-header"
	}

	localMarker, _ := os.ReadFile(localMarkerPath)
	localStamp := strings.TrimSpace(string(localMarker))

	mmdbStat, statErr := os.Stat(localMmdbPath)
	if statErr != nil || remoteModTime != localStamp {
		log.Log(log.Info, "Downloading fresh MaxMind DB for %s ...", dbName)

		tmpArchivePath := filepath.Join(baseDir, dbName+".tar.gz")
		err = downloadDatabase(remoteURL, accountID, licenseKey, tmpArchivePath)
		if err != nil {
			if st, statErr := os.Stat(localMmdbPath); statErr == nil && !st.IsDir() {
				log.Log(log.Warn, "%s download failed; keeping existing local copy: %v", dbName, err)
				return nil
			}
			return fmt.Errorf("download of %s failed: %w", dbName, err)
		}

		if err := extractTarGz(tmpArchivePath, baseDir); err != nil {
			return fmt.Errorf("extract error for %s: %w", dbName, err)
		}

		extractedMmdb, findErr := findExtractedMmdb(baseDir, editionID)
		if findErr != nil {
			return fmt.Errorf("cannot find extracted mmdb for %s: %w", dbName, findErr)
		}

		if err := os.RemoveAll(localMmdbPath); err != nil {
			log.Log(log.Error, "Could not remove old file %s: %v", localMmdbPath, err)
		}
		if renameErr := os.Rename(extractedMmdb, localMmdbPath); renameErr != nil {
			return fmt.Errorf("rename to final mmdb %s failed: %w", localMmdbPath, renameErr)
		}

		if err := os.Remove(tmpArchivePath); err != nil {
			log.Log(log.Error, "Could not remove archive file %s: %v", tmpArchivePath, err)
		}

		cleanupExtractedDirs(baseDir, editionID)

		os.WriteFile(localMarkerPath, []byte(remoteModTime), 0644)
	} else {
		log.Log(log.Info, "Local %s is up-to-date, local stamp = %s, remote = %s, size: %d",
			dbName, localStamp, remoteModTime, mmdbStat.Size())
	}
	return nil
}

func getRemoteLastModified(url, accountID, licenseKey string) (string, error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(accountID, licenseKey)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HEAD status: %d, %s", resp.StatusCode, resp.Status)
	}

	return resp.Header.Get("Last-Modified"), nil
}

func downloadDatabase(url, accountID, licenseKey, outPath string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(accountID, licenseKey)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET status: %d, %s", resp.StatusCode, resp.Status)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func findExtractedMmdb(baseDir, editionID string) (string, error) {
	pattern := fmt.Sprintf(`^%s_(\d{8})$`, editionID)
	re := regexp.MustCompile(pattern)

	dEntries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", err
	}
	for _, de := range dEntries {
		if de.IsDir() && re.MatchString(de.Name()) {
			subDirPath := filepath.Join(baseDir, de.Name())
			foundMmdb, errWalk := walkForMmdb(subDirPath)
			if errWalk != nil {
				log.Log(log.Error, "Error scanning folder %s: %v", subDirPath, errWalk)
				continue
			}
			if foundMmdb != "" {
				return foundMmdb, nil
			}
		}
	}
	return "", fmt.Errorf("no extracted mmdb found for %s in %s", editionID, baseDir)
}

func walkForMmdb(path string) (string, error) {
	var found string
	err := filepath.Walk(path, func(fp string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".mmdb") {
			found = fp
			return io.EOF
		}
		return nil
	})
	if err == io.EOF {
		return found, nil
	}
	return found, err
}

func cleanupExtractedDirs(baseDir, editionID string) {
	entries, _ := os.ReadDir(baseDir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, editionID+"_") {
			fullPath := filepath.Join(baseDir, name)
			os.RemoveAll(fullPath)
		}
	}
}

func extractTarGz(tarGzPath, destDir string) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tarReader := tar.NewReader(gzr)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		outPath := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(outPath, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(outFile, tarReader)
			outFile.Close()
			if copyErr != nil {
				return copyErr
			}
		default:
			// skip symlinks, etc
		}
	}
	return nil
}
