package linuxaudit

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/qiniu/logkit/parser/config"
	. "github.com/qiniu/logkit/utils/models"
)

func TestParse(t *testing.T) {
	tests := []struct {
		s          []string
		expectData []Data
	}{
		{
			expectData: []Data{},
		},
		{
			s: []string{`type=SYSCALL msg=audit(1364481363.243:24287): arch=c000003e syscall=2 success=no exit=-13 a0=7fffd19c5592 a1=0    a2=7fffd19c4b50`,
				`type=CWD msg='op=PAM:secret test1="a" res=success'
					cwd="/home/shadowman" `,
				`type=PATH msg=audit(1364481363.243:24287): item=0 name="/etc/ssh/sshd_config" inode=409248 dev=fd:00 dev=system_u:object_r:etc_t:s0`},
			expectData: []Data{
				{
					"arch":          "c000003e",
					"type":          "SYSCALL",
					"msg_timestamp": "1364481363.243",
					"msg_id":        "24287",
					"syscall":       "2",
					"success":       "no",
					"exit":          "-13",
					"a0":            "7fffd19c5592",
					"a1":            "0",
					"a2":            "7fffd19c4b50",
				},
				{
					"type": "CWD",
					"cwd":  "/home/shadowman",
					"msg":  Data{"op": "PAM:secret", "test1": "a", "res": "success"},
				},
				{
					"type":          "PATH",
					"msg_timestamp": "1364481363.243",
					"msg_id":        "24287",
					"item":          "0",
					"name":          "/etc/ssh/sshd_config",
					"inode":         "409248",
					"dev":           "fd:00",
					"dev_1":         "system_u:object_r:etc_t:s0",
				},
			},
		},
	}
	l := Parser{
		name: TypeLinuxAudit,
	}
	for _, tt := range tests {
		got, err := l.Parse(tt.s)
		if c, ok := err.(*StatsError); ok {
			err = errors.New(c.LastError)
			assert.Equal(t, int64(0), c.Errors)
		}

		for i, m := range got {
			assert.Equal(t, tt.expectData[i], m)
		}
	}
}

func Test_parseLine(t *testing.T) {
	tests := []struct {
		line       string
		expectData Data
	}{
		{
			expectData: Data{},
		},
		{
			line: `type=SYSCALL msg=audit(1364481363.243:24287): arch=c000003e syscall=2 success=no exit=-13 a0=7fffd19c5592 a1=0    a2=7fffd19c4b50`,
			expectData: Data{
				"arch":          "c000003e",
				"type":          "SYSCALL",
				"msg_timestamp": "2013-03-28T14:36:03.243Z",
				"msg_id":        "24287",
				"syscall":       "2",
				"success":       "no",
				"exit":          "-13",
				"a0":            "7fffd19c5592",
				"a1":            "0",
				"a2":            "7fffd19c4b50",
			},
		},
		{
			line: `type=CWD msg='op=PAM:secret test1="a" res=success'
					cwd="/home/shadowman" `,
			expectData: Data{
				"type": "CWD",
				"cwd":  "/home/shadowman",
				"msg":  Data{"op": "PAM:secret", "test1": "a", "res": "success"},
			},
		},
		{
			line: `type=PATH msg=audit(1364481363.243:24287): item=0 name="/etc/ssh/sshd_config" inode=409248 dev=fd:00 dev=system_u:object_r:etc_t:s0`,
			expectData: Data{
				"type":          "PATH",
				"msg_timestamp": "2013-03-28T14:36:03.243Z",
				"msg_id":        "24287",
				"item":          "0",
				"name":          "/etc/ssh/sshd_config",
				"inode":         "409248",
				"dev":           "fd:00",
				"dev_1":         "system_u:object_r:etc_t:s0",
			},
		},
	}
	l := Parser{
		name: TypeLinuxAudit,
	}
	for _, tt := range tests {
		got, err := l.parse(tt.line)
		assert.Nil(t, err)
		assert.Equal(t, len(tt.expectData), len(got))
		for i, m := range got {
			assert.Equal(t, tt.expectData[i], m)
		}
	}
}

func Test_processSpace(t *testing.T) {
	tests := []struct {
		key    string
		line   string
		data   Data
		expect Data
	}{
		{
			data: Data{},
		},
		{
			key:    "a",
			line:   "b",
			data:   Data{},
			expect: Data{"a": "b"},
		},
		{
			key:    "msg",
			line:   "audit(111111:222)",
			data:   Data{},
			expect: Data{"msg_timestamp": "2005-03-18T01:40:00Z", "msg_id": "222"},
		},
	}

	for _, test := range tests {
		processSpace(test.key, test.line, test.data)
		assert.EqualValues(t, len(test.expect), len(test.data))
		for key, value := range test.expect {
			val, ok := test.data[key]
			assert.True(t, ok)
			assert.EqualValues(t, value, val)
		}
	}
}

func Test_getTimestampID(t *testing.T) {
	tests := []struct {
		line    string
		data    Data
		success bool
	}{
		{
			data: Data{},
		},
		{
			line: "a",
			data: Data{},
		},
		{
			line:    "audit(111111:222)",
			data:    Data{},
			success: true,
		},
	}

	for _, test := range tests {
		actual := getTimestampID(test.line, test.data)
		assert.EqualValues(t, test.success, actual)
		if actual {
			_, ok := test.data["msg_timestamp"]
			assert.True(t, ok)
			_, ok = test.data["msg_id"]
			assert.True(t, ok)
		}
	}
}

func Test_setData(t *testing.T) {
	tests := []struct {
		key    string
		line   string
		data   Data
		expect Data
	}{
		{
			data: Data{},
		},
		{
			key:    "a",
			line:   "b",
			data:   Data{},
			expect: Data{"a": "b"},
		},
		{
			key:    "msg",
			line:   "audit(111111:222)",
			data:   Data{},
			expect: Data{"msg": "audit(111111:222)"},
		},
	}

	for _, test := range tests {
		setData(test.key, test.line, test.data)
		assert.EqualValues(t, len(test.expect), len(test.data))
		for key, value := range test.expect {
			val, ok := test.data[key]
			assert.True(t, ok)
			assert.EqualValues(t, value, val)
		}
	}
}
