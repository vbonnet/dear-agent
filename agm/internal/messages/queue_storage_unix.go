//go:build darwin || linux

package messages

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type queueStorageLeafSnapshot struct {
	spec     queueStorageLeafSpec
	identity queueStorageIdentity
	fd       int
}

func prepareMessageQueueStorage(homeDir string) (*messageQueueStorage, error) {
	if !filepath.IsAbs(homeDir) || filepath.Clean(homeDir) != homeDir {
		return nil, unsafeQueueStorageError("home directory", "is not an absolute clean path")
	}

	storage := newMessageQueueStorage()
	storage.homePath = homeDir
	storage.rootPath = filepath.Join(homeDir, ".config", "agm")
	storage.dbPath = filepath.Join(storage.rootPath, queueDatabaseLeaf)
	storage.dbLeaf = queueDatabaseLeaf
	storage.uid = queueStorageEffectiveUID()
	storage.productionChain = true

	var err error
	storage.homeFD, storage.homeIdentity, err = openQueueStorageDirectory(
		homeDir,
		"home directory",
		storage.uid,
	)
	if err != nil {
		return nil, err
	}

	storage.configFD, storage.configIdentity, err = openOrCreateQueueStorageChildDirectory(
		storage.homeFD,
		".config",
		"configuration directory",
		storage.uid,
	)
	if err != nil {
		return nil, errors.Join(err, storage.Close())
	}

	storage.rootFD, storage.rootIdentity, err = openOrCreateQueueStorageChildDirectory(
		storage.configFD,
		"agm",
		"AGM directory",
		storage.uid,
	)
	if err != nil {
		return nil, errors.Join(err, storage.Close())
	}

	if err := storage.revalidateDirectoryChain(false); err != nil {
		return nil, errors.Join(err, storage.Close())
	}
	return storage, nil
}

func newMessageQueueStorage() *messageQueueStorage {
	return &messageQueueStorage{
		homeFD:   queueStorageInvalidFD,
		configFD: queueStorageInvalidFD,
		rootFD:   queueStorageInvalidFD,
	}
}

func queueStorageEffectiveUID() uint32 {
	// os.Geteuid reports the platform uid_t through int; Darwin and Linux use
	// non-negative 32-bit user IDs.
	return uint32(os.Geteuid()) //nolint:gosec // guarded by the Unix uid_t contract above
}

func openQueueStorageDirectory(
	path string,
	artifact string,
	expectedUID uint32,
) (int, queueStorageIdentity, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return queueStorageInvalidFD, queueStorageIdentity{},
			unsafeQueueStorageError(artifact, "could not be admitted")
	}

	identity, err := validateQueueStorageDirectoryDescriptor(fd, artifact, expectedUID)
	if err != nil {
		_ = unix.Close(fd)
		return queueStorageInvalidFD, queueStorageIdentity{}, err
	}
	return fd, identity, nil
}

func openOrCreateQueueStorageChildDirectory(
	parentFD int,
	name string,
	artifact string,
	expectedUID uint32,
) (int, queueStorageIdentity, error) {
	created := false
	for range queueStorageAdmissionLimit {
		fd, err := unix.Openat(
			parentFD,
			name,
			unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC,
			0,
		)
		if err == nil {
			identity, validationErr := validateQueueStorageDirectoryDescriptor(fd, artifact, expectedUID)
			if validationErr != nil {
				_ = unix.Close(fd)
				return queueStorageInvalidFD, queueStorageIdentity{}, validationErr
			}
			if created {
				if chmodErr := chmodAndVerifyQueueStorageDirectory(fd, artifact, expectedUID, identity); chmodErr != nil {
					_ = unix.Close(fd)
					return queueStorageInvalidFD, queueStorageIdentity{}, chmodErr
				}
			}
			return fd, identity, nil
		}
		if !errors.Is(err, unix.ENOENT) {
			return queueStorageInvalidFD, queueStorageIdentity{},
				unsafeQueueStorageError(artifact, "could not be admitted")
		}

		err = unix.Mkdirat(parentFD, name, 0o700)
		if err == nil {
			created = true
			continue
		}
		if !errors.Is(err, unix.EEXIST) {
			return queueStorageInvalidFD, queueStorageIdentity{},
				unsafeQueueStorageError(artifact, "could not be created privately")
		}
	}

	return queueStorageInvalidFD, queueStorageIdentity{},
		unsafeQueueStorageError(artifact, "changed too often during admission")
}

func validateQueueStorageDirectoryDescriptor(
	fd int,
	artifact string,
	expectedUID uint32,
) (queueStorageIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return queueStorageIdentity{}, unsafeQueueStorageError(artifact, "metadata could not be read")
	}
	if err := validateQueueStorageDirectoryStat(artifact, &stat, expectedUID); err != nil {
		return queueStorageIdentity{}, err
	}
	return queueStorageIdentityFromStat(&stat), nil
}

func validateQueueStorageDirectoryStat(artifact string, stat *unix.Stat_t, expectedUID uint32) error {
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return unsafeQueueStorageError(artifact, "is not a directory")
	}
	if err := validateQueueStorageOwner(artifact, stat.Uid, expectedUID); err != nil {
		return err
	}
	if stat.Mode&0o022 != 0 {
		return unsafeQueueStorageError(artifact, "is writable by another account")
	}
	return nil
}

func (s *messageQueueStorage) classifyQueueStorageLeaves() (
	[]queueStorageLeafSnapshot,
	bool,
	bool,
	error,
) {
	leaves := make([]queueStorageLeafSnapshot, 0, len(queueStorageLeafSpecs))
	mainPresent := false
	for _, baseSpec := range queueStorageLeafSpecs {
		spec := baseSpec
		spec.name = s.leafName(spec)
		var stat unix.Stat_t
		err := unix.Fstatat(s.rootFD, spec.name, &stat, unix.AT_SYMLINK_NOFOLLOW)
		if errors.Is(err, unix.ENOENT) {
			continue
		}
		if err != nil {
			return nil, false, false, unsafeQueueStorageError(spec.artifact, "metadata could not be read")
		}
		if !spec.main && uint64(stat.Nlink) == 0 {
			return nil, false, true, nil
		}
		if err := validateQueueStorageLeafStat(spec.artifact, &stat, s.uid, false); err != nil {
			return nil, false, false, err
		}
		leaves = append(leaves, queueStorageLeafSnapshot{
			spec:     spec,
			identity: queueStorageIdentityFromStat(&stat),
			fd:       queueStorageInvalidFD,
		})
		mainPresent = mainPresent || spec.main
	}
	return leaves, mainPresent, false, nil
}

func (s *messageQueueStorage) leafName(spec queueStorageLeafSpec) string {
	if spec.main {
		return s.dbLeaf
	}
	return s.dbLeaf + spec.name[len(queueDatabaseLeaf):]
}

func validateQueueStorageLeafStat(
	artifact string,
	stat *unix.Stat_t,
	expectedUID uint32,
	requirePrivateMode bool,
) error {
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return unsafeQueueStorageError(artifact, "is not a regular file")
	}
	if err := validateQueueStorageOwner(artifact, stat.Uid, expectedUID); err != nil {
		return err
	}
	if uint64(stat.Nlink) != 1 {
		return unsafeQueueStorageError(artifact, "does not have exactly one link")
	}
	if requirePrivateMode && stat.Mode&0o777 != 0o600 {
		return unsafeQueueStorageError(artifact, "does not have mode 0600")
	}
	return nil
}

func (s *messageQueueStorage) admitAndRepairQueueStorageLeaves(
	leaves []queueStorageLeafSnapshot,
) (bool, error) {
	defer closeQueueStorageLeafDescriptors(leaves)

	retry, err := s.openAndValidateQueueStorageLeaves(leaves)
	if err != nil || retry {
		return retry, err
	}
	if err := s.revalidateDirectoryChain(false); err != nil {
		return false, err
	}
	if err := chmodAndVerifyQueueStorageDirectory(
		s.rootFD,
		"AGM directory",
		s.uid,
		s.rootIdentity,
	); err != nil {
		return false, err
	}
	if err := s.revalidateDirectoryChain(true); err != nil {
		return false, err
	}
	// Sealing the root closes the path-only race for another UID, but an
	// attacker may have added a hard link after the earlier complete admission
	// pass and immediately before the chmod. Revalidate the entire held set
	// again after the seal and before mutating even the first leaf.
	retry, err = s.revalidateSealedQueueStorageLeaves(leaves)
	if err != nil || retry {
		return retry, err
	}
	return s.repairQueueStorageLeaves(leaves)
}

func (s *messageQueueStorage) openAndValidateQueueStorageLeaves(
	leaves []queueStorageLeafSnapshot,
) (bool, error) {
	for i := range leaves {
		leaf := &leaves[i]
		fd, err := unix.Openat(
			s.rootFD,
			leaf.spec.name,
			unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC,
			0,
		)
		if errors.Is(err, unix.ENOENT) {
			return true, nil
		}
		if err != nil {
			return false, unsafeQueueStorageError(leaf.spec.artifact, "could not be descriptor-admitted")
		}
		leaf.fd = fd

		var descriptorStat unix.Stat_t
		if err := unix.Fstat(fd, &descriptorStat); err != nil {
			return false, unsafeQueueStorageError(leaf.spec.artifact, "descriptor metadata could not be read")
		}
		if !leaf.spec.main && uint64(descriptorStat.Nlink) == 0 {
			// SQLite can unlink a WAL, SHM, or rollback journal between the
			// no-follow snapshot and descriptor admission. Discard the entire
			// snapshot rather than treating its now-unlinked descriptor as a
			// candidate for mutation.
			return true, nil
		}
		if err := validateQueueStorageLeafStat(leaf.spec.artifact, &descriptorStat, s.uid, false); err != nil {
			return false, err
		}
		if queueStorageIdentityFromStat(&descriptorStat) != leaf.identity {
			return false, unsafeQueueStorageError(leaf.spec.artifact, "changed identity during admission")
		}

		visibleStat, retry, err := s.visibleQueueStorageLeafStat(leaf.spec, false)
		if err != nil {
			return false, err
		}
		if retry {
			return true, nil
		}
		if queueStorageIdentityFromStat(&visibleStat) != leaf.identity {
			return false, unsafeQueueStorageError(leaf.spec.artifact, "changed identity during admission")
		}
	}
	return false, nil
}

func (s *messageQueueStorage) repairQueueStorageLeaves(
	leaves []queueStorageLeafSnapshot,
) (bool, error) {
	for i := range leaves {
		leaf := &leaves[i]
		if err := unix.Fchmod(leaf.fd, 0o600); err != nil {
			return false, unsafeQueueStorageError(leaf.spec.artifact, "could not be made private")
		}

		var descriptorStat unix.Stat_t
		if err := unix.Fstat(leaf.fd, &descriptorStat); err != nil {
			return false, unsafeQueueStorageError(leaf.spec.artifact, "private mode could not be verified")
		}
		if !leaf.spec.main && uint64(descriptorStat.Nlink) == 0 {
			return true, nil
		}
		if err := validateQueueStorageLeafStat(leaf.spec.artifact, &descriptorStat, s.uid, true); err != nil {
			return false, err
		}
		if queueStorageIdentityFromStat(&descriptorStat) != leaf.identity {
			return false, unsafeQueueStorageError(leaf.spec.artifact, "changed identity during repair")
		}

		visibleStat, retry, err := s.visibleQueueStorageLeafStat(leaf.spec, true)
		if err != nil {
			return false, err
		}
		if retry {
			return true, nil
		}
		if queueStorageIdentityFromStat(&visibleStat) != leaf.identity {
			return false, unsafeQueueStorageError(leaf.spec.artifact, "changed identity during repair")
		}
		if leaf.spec.main {
			s.mainIdentity = leaf.identity
		}
	}
	return false, nil
}

func (s *messageQueueStorage) revalidateSealedQueueStorageLeaves(
	leaves []queueStorageLeafSnapshot,
) (bool, error) {
	for i := range leaves {
		leaf := &leaves[i]
		if leaf.fd == queueStorageInvalidFD {
			return false, unsafeQueueStorageError(leaf.spec.artifact, "lost its admitted descriptor")
		}

		var descriptorStat unix.Stat_t
		if err := unix.Fstat(leaf.fd, &descriptorStat); err != nil {
			return false, unsafeQueueStorageError(leaf.spec.artifact, "sealed metadata could not be read")
		}
		if !leaf.spec.main && uint64(descriptorStat.Nlink) == 0 {
			return true, nil
		}
		if err := validateQueueStorageLeafStat(leaf.spec.artifact, &descriptorStat, s.uid, false); err != nil {
			return false, err
		}
		if queueStorageIdentityFromStat(&descriptorStat) != leaf.identity {
			return false, unsafeQueueStorageError(leaf.spec.artifact, "changed identity before repair")
		}

		visibleStat, retry, err := s.visibleQueueStorageLeafStat(leaf.spec, false)
		if err != nil {
			return false, err
		}
		if retry {
			return true, nil
		}
		if queueStorageIdentityFromStat(&visibleStat) != leaf.identity {
			return false, unsafeQueueStorageError(leaf.spec.artifact, "changed identity before repair")
		}
	}
	return false, nil
}

func (s *messageQueueStorage) visibleQueueStorageLeafStat(
	spec queueStorageLeafSpec,
	requirePrivateMode bool,
) (unix.Stat_t, bool, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(s.rootFD, spec.name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, unix.ENOENT) {
		return stat, true, nil
	}
	if err != nil {
		return stat, false, unsafeQueueStorageError(spec.artifact, "visible metadata could not be read")
	}
	if !spec.main && uint64(stat.Nlink) == 0 {
		return stat, true, nil
	}
	if err := validateQueueStorageLeafStat(spec.artifact, &stat, s.uid, requirePrivateMode); err != nil {
		return stat, false, err
	}
	return stat, false, nil
}

func (s *messageQueueStorage) revalidateDirectoryChain(requirePrivateRoot bool) error {
	if s == nil {
		return unsafeQueueStorageError("storage capability", "is absent")
	}
	if s.productionChain {
		if err := revalidateQueueStorageDirectoryDescriptor(
			s.homeFD,
			"home directory",
			s.uid,
			s.homeIdentity,
			false,
		); err != nil {
			return err
		}
		visibleHomeFD, visibleHomeIdentity, err := openQueueStorageDirectory(
			s.homePath,
			"home directory",
			s.uid,
		)
		if err != nil {
			return err
		}
		_ = unix.Close(visibleHomeFD)
		if visibleHomeIdentity != s.homeIdentity {
			return unsafeQueueStorageError("home directory", "changed identity")
		}

		if err := s.revalidateVisibleChildDirectory(
			s.homeFD,
			".config",
			"configuration directory",
			s.configIdentity,
			false,
		); err != nil {
			return err
		}
		if err := revalidateQueueStorageDirectoryDescriptor(
			s.configFD,
			"configuration directory",
			s.uid,
			s.configIdentity,
			false,
		); err != nil {
			return err
		}
		if err := s.revalidateVisibleChildDirectory(
			s.configFD,
			"agm",
			"AGM directory",
			s.rootIdentity,
			requirePrivateRoot,
		); err != nil {
			return err
		}
	} else {
		visibleRootFD, visibleRootIdentity, err := openQueueStorageDirectory(
			s.rootPath,
			"queue directory",
			s.uid,
		)
		if err != nil {
			return err
		}
		if requirePrivateRoot {
			var stat unix.Stat_t
			if statErr := unix.Fstat(visibleRootFD, &stat); statErr != nil || stat.Mode&0o777 != 0o700 {
				_ = unix.Close(visibleRootFD)
				return unsafeQueueStorageError("queue directory", "does not have mode 0700")
			}
		}
		_ = unix.Close(visibleRootFD)
		if visibleRootIdentity != s.rootIdentity {
			return unsafeQueueStorageError("queue directory", "changed identity")
		}
	}

	return revalidateQueueStorageDirectoryDescriptor(
		s.rootFD,
		"AGM directory",
		s.uid,
		s.rootIdentity,
		requirePrivateRoot,
	)
}

func (s *messageQueueStorage) revalidateVisibleChildDirectory(
	parentFD int,
	name string,
	artifact string,
	expectedIdentity queueStorageIdentity,
	requirePrivateMode bool,
) error {
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return unsafeQueueStorageError(artifact, "visible metadata could not be read")
	}
	if err := validateQueueStorageDirectoryStat(artifact, &stat, s.uid); err != nil {
		return err
	}
	if requirePrivateMode && stat.Mode&0o777 != 0o700 {
		return unsafeQueueStorageError(artifact, "does not have mode 0700")
	}
	if queueStorageIdentityFromStat(&stat) != expectedIdentity {
		return unsafeQueueStorageError(artifact, "changed identity")
	}
	return nil
}

func revalidateQueueStorageDirectoryDescriptor(
	fd int,
	artifact string,
	expectedUID uint32,
	expectedIdentity queueStorageIdentity,
	requirePrivateMode bool,
) error {
	identity, err := validateQueueStorageDirectoryDescriptor(fd, artifact, expectedUID)
	if err != nil {
		return err
	}
	if identity != expectedIdentity {
		return unsafeQueueStorageError(artifact, "changed identity")
	}
	if requirePrivateMode {
		var stat unix.Stat_t
		if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&0o777 != 0o700 {
			return unsafeQueueStorageError(artifact, "does not have mode 0700")
		}
	}
	return nil
}

func chmodAndVerifyQueueStorageDirectory(
	fd int,
	artifact string,
	expectedUID uint32,
	expectedIdentity queueStorageIdentity,
) error {
	if err := unix.Fchmod(fd, 0o700); err != nil {
		return unsafeQueueStorageError(artifact, "could not be made private")
	}
	return revalidateQueueStorageDirectoryDescriptor(fd, artifact, expectedUID, expectedIdentity, true)
}

func queueStorageIdentityFromStat(stat *unix.Stat_t) queueStorageIdentity {
	return queueStorageIdentity{
		device: uint64(stat.Dev), //nolint:gosec,nolintlint // Darwin dev_t is signed; equality uses its stable bit pattern
		inode:  uint64(stat.Ino),
	}
}

func closeQueueStorageLeafDescriptors(leaves []queueStorageLeafSnapshot) {
	for i := range leaves {
		if leaves[i].fd != queueStorageInvalidFD {
			_ = unix.Close(leaves[i].fd)
			leaves[i].fd = queueStorageInvalidFD
		}
	}
}

func (s *messageQueueStorage) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true

	var closeErrors []error
	for _, descriptor := range []struct {
		fd       *int
		artifact string
	}{
		{fd: &s.rootFD, artifact: "AGM directory"},
		{fd: &s.configFD, artifact: "configuration directory"},
		{fd: &s.homeFD, artifact: "home directory"},
	} {
		if *descriptor.fd == queueStorageInvalidFD {
			continue
		}
		if err := unix.Close(*descriptor.fd); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close %s descriptor", descriptor.artifact))
		}
		*descriptor.fd = queueStorageInvalidFD
	}
	return errors.Join(closeErrors...)
}
