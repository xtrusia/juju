// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	sshtesting "github.com/juju/utils/v3/ssh/testing"
	gc "gopkg.in/check.v1"

	keymanagerserver "github.com/juju/juju/apiserver/facades/client/keymanager"
	keymanagertesting "github.com/juju/juju/apiserver/facades/client/keymanager/testing"
	"github.com/juju/juju/juju/osenv"
	jujutesting "github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
)

type SSHKeysSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&SSHKeysSuite{})

func (s *SSHKeysSuite) assertHelpOutput(c *gc.C, cmd, args string) {
	if args != "" {
		args = " " + args
	}
	expected := fmt.Sprintf("Usage: juju %s [options]%s", cmd, args)
	out := badrun(c, 0, cmd, "--help")
	lines := strings.Split(out, "\n")
	c.Assert(lines[0], gc.Equals, expected)
}

func (s *SSHKeysSuite) TestHelpList(c *gc.C) {
	s.assertHelpOutput(c, "ssh-keys", "")
}

func (s *SSHKeysSuite) TestHelpAdd(c *gc.C) {
	s.assertHelpOutput(c, "add-ssh-key", "<ssh key> ...")
}

func (s *SSHKeysSuite) TestHelpRemove(c *gc.C) {
	s.assertHelpOutput(c, "remove-ssh-key", "<ssh key id> ...")
}

func (s *SSHKeysSuite) TestHelpImport(c *gc.C) {
	s.assertHelpOutput(c, "import-ssh-key", "<lp|gh>:<user identity> ...")
}

type keySuiteBase struct {
	jujutesting.JujuConnSuite
	coretesting.CmdBlockHelper
}

func (s *keySuiteBase) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.PatchEnvironment(osenv.JujuModelEnvKey, "controller")
}

func (s *keySuiteBase) SetUpTest(c *gc.C) {
	if runtime.GOOS == "darwin" {
		c.Skip("Mongo failures on macOS")
	}
	s.JujuConnSuite.SetUpTest(c)
	s.CmdBlockHelper = coretesting.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

func (s *keySuiteBase) setAuthorizedKeys(c *gc.C, keys ...string) {
	keyString := strings.Join(keys, "\n")
	err := s.Model.UpdateModelConfig(map[string]interface{}{"authorized-keys": keyString}, nil)
	c.Assert(err, jc.ErrorIsNil)
	envConfig, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envConfig.AuthorizedKeys(), gc.Equals, keyString)
}

func (s *keySuiteBase) assertEnvironKeys(c *gc.C, expected ...string) {
	envConfig, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	keys := envConfig.AuthorizedKeys()
	c.Assert(keys, gc.Equals, strings.Join(expected, "\n"))
}

type ListKeysSuite struct {
	keySuiteBase
}

var _ = gc.Suite(&ListKeysSuite{})

func (s *ListKeysSuite) TestListKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorizedKeys(c, key1, key2)

	context, err := cmdtesting.RunCommand(c, NewListKeysCommand())
	c.Assert(err, jc.ErrorIsNil)
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Matches, "Keys used in model: controller\n.*\\(user@host\\)\n.*\\(another@host\\)")
}

func (s *ListKeysSuite) TestListKeysWithModelUUID(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorizedKeys(c, key1, key2)

	context, err := cmdtesting.RunCommand(c, NewListKeysCommand(), "-m", s.Model.UUID())
	c.Assert(err, jc.ErrorIsNil)
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Matches,
		fmt.Sprintf("Keys used in model: %s\n.*\\(user@host\\)\n.*\\(another@host\\)", s.Model.UUID()))
}

func (s *ListKeysSuite) TestListFullKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorizedKeys(c, key1, key2)

	context, err := cmdtesting.RunCommand(c, NewListKeysCommand(), "--full")
	c.Assert(err, jc.ErrorIsNil)
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Matches, "Keys used in model: controller\n.*user@host\n.*another@host")
}

func (s *ListKeysSuite) TestTooManyArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, NewListKeysCommand(), "foo")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["foo"\]`)
}

type AddKeySuite struct {
	keySuiteBase
}

var _ = gc.Suite(&AddKeySuite{})

// TestAddKey reports all per-key errors and applies successful additions.
func (s *AddKeySuite) TestAddKey(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorizedKeys(c, key1)

	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	context, err := cmdtesting.RunCommand(c, NewAddKeysCommand(), "invalid-key", "another-invalid-key", key2)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	addErrors := cmdtesting.Stderr(context)
	c.Check(strings.Count(addErrors, "cannot add key"), gc.Equals, 2)
	c.Check(strings.Contains(addErrors, `cannot add key "invalid-key":`), jc.IsTrue)
	c.Check(strings.Contains(addErrors, `cannot add key "another-invalid-key":`), jc.IsTrue)
	s.assertEnvironKeys(c, key1, key2)
}

// TestAddKeysSucceeds returns nil when every key is added.
func (s *AddKeySuite) TestAddKeysSucceeds(c *gc.C) {
	s.setAuthorizedKeys(c)
	key := sshtesting.ValidKeyTwo.Key + " another@host"
	_, err := cmdtesting.RunCommand(c, NewAddKeysCommand(), key)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironKeys(c, key)
}

func (s *AddKeySuite) TestBlockAddKey(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorizedKeys(c, key1)

	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	// Block operation
	s.BlockAllChanges(c, "TestBlockAddKey")
	_, err := cmdtesting.RunCommand(c, NewAddKeysCommand(), key2, "invalid-key")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestBlockAddKey.*")
}

type RemoveKeySuite struct {
	keySuiteBase
}

var _ = gc.Suite(&RemoveKeySuite{})

// TestRemoveKeys reports all per-key errors and applies successful removals.
func (s *RemoveKeySuite) TestRemoveKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorizedKeys(c, key1, key2)

	context, err := cmdtesting.RunCommand(c, NewRemoveKeysCommand(),
		"invalid-key", "another-invalid-key", sshtesting.ValidKeyTwo.Fingerprint)
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	removeErrors := cmdtesting.Stderr(context)
	c.Check(strings.Count(removeErrors, "cannot remove key id"), gc.Equals, 2)
	c.Check(strings.Contains(removeErrors, `cannot remove key id "invalid-key":`), jc.IsTrue)
	c.Check(strings.Contains(removeErrors, `cannot remove key id "another-invalid-key":`), jc.IsTrue)
	s.assertEnvironKeys(c, key1)
}

// TestRemoveKeysSucceeds returns nil when every key is removed.
func (s *RemoveKeySuite) TestRemoveKeysSucceeds(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorizedKeys(c, key1, key2)
	_, err := cmdtesting.RunCommand(c, NewRemoveKeysCommand(), sshtesting.ValidKeyOne.Fingerprint)
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironKeys(c, key2)
}

func (s *RemoveKeySuite) TestBlockRemoveKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorizedKeys(c, key1, key2)

	// Block operation
	s.BlockAllChanges(c, "TestBlockRemoveKeys")
	_, err := cmdtesting.RunCommand(c, NewRemoveKeysCommand(),
		sshtesting.ValidKeyTwo.Fingerprint, "invalid-key")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestBlockRemoveKeys.*")
}

type ImportKeySuite struct {
	keySuiteBase
}

var _ = gc.Suite(&ImportKeySuite{})

func (s *ImportKeySuite) SetUpTest(c *gc.C) {
	s.keySuiteBase.SetUpTest(c)
	s.PatchValue(&keymanagerserver.RunSSHImportId, keymanagertesting.FakeImport)
}

// TestImportKeys reports all per-key errors and applies successful imports.
func (s *ImportKeySuite) TestImportKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorizedKeys(c, key1)

	context, err := cmdtesting.RunCommand(c, NewImportKeysCommand(), "lp:invalid-key", "lp:invalid-key-2", "lp:validuser")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	importErrors := cmdtesting.Stderr(context)
	c.Check(strings.Count(importErrors, "cannot import key id"), gc.Equals, 2)
	c.Check(strings.Contains(importErrors, `cannot import key id "lp:invalid-key":`), jc.IsTrue)
	c.Check(strings.Contains(importErrors, `cannot import key id "lp:invalid-key-2":`), jc.IsTrue)
	s.assertEnvironKeys(c, key1, sshtesting.ValidKeyThree.Key)
}

// TestImportKeysSucceeds returns nil when every key is imported.
func (s *ImportKeySuite) TestImportKeysSucceeds(c *gc.C) {
	s.setAuthorizedKeys(c)
	_, err := cmdtesting.RunCommand(c, NewImportKeysCommand(), "lp:validuser")
	c.Assert(err, jc.ErrorIsNil)
	s.assertEnvironKeys(c, sshtesting.ValidKeyThree.Key)
}

func (s *ImportKeySuite) TestBlockImportKeys(c *gc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorizedKeys(c, key1)

	// Block operation
	s.BlockAllChanges(c, "TestBlockImportKeys")
	_, err := cmdtesting.RunCommand(c, NewImportKeysCommand(), "lp:validuser", "lp:invalid-key")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestBlockImportKeys.*")
}
