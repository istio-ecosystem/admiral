package registry

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"reflect"
	"testing"
)

func TestGet(t *testing.T) {
	var (
		record1       = "record1"
		record2       = "record2"
		record2Config = &IdentityConfig{
			IdentityName: record2,
		}
		emptyHandler       = NewConfigCache()
		handlerWithRecord2 = NewConfigCache()
	)
	handlerWithRecord2.Update(record2, record2Config, NewConfigMapWriter(nil, logrus.WithFields(logrus.Fields{
		"type": "storeWorkloadData",
	})))
	cases := []struct {
		name         string
		identityName string
		handler      *cacheHandler
		expConfig    *IdentityConfig
		expErr       error
	}{
		{
			name: "Given there is no record for '" + record1 + "', " +
				"When the Get function is called, " +
				"It should return an error",
			identityName: record1,
			handler:      emptyHandler,
			expErr:       fmt.Errorf("record not found"),
		},
		{
			name: "Given there is a record for '" + record2 + "', " +
				"When the Get function is called, " +
				"It should return the record",
			identityName: record2,
			handler:      handlerWithRecord2,
			expConfig:    record2Config,
			expErr:       nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			config, err := c.handler.Get(c.identityName)
			if !reflect.DeepEqual(err, c.expErr) {
				t.Errorf("want=%v, got=%v", c.expErr, err)
			}
			if !reflect.DeepEqual(config, c.expConfig) {
				t.Errorf("want=%v, got=%v", c.expConfig, config)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	var (
		record1       = "record1"
		record1Config = &IdentityConfig{
			IdentityName: record1,
		}
		emptyHandler = NewConfigCache()
	)
	cases := []struct {
		name         string
		identityName string
		handler      *cacheHandler
		config       *IdentityConfig
		expErr       error
	}{
		{
			name: "Given there is no record for '" + record1 + "', " +
				"When the Get function is called, " +
				"It should cache the config",
			identityName: record1,
			config:       record1Config,
			handler:      emptyHandler,
			expErr:       nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.handler.Update(c.identityName, c.config, NewConfigMapWriter(nil, logrus.WithFields(logrus.Fields{
				"type": "storeWorkloadData",
			})))
			if !reflect.DeepEqual(err, c.expErr) {
				t.Errorf("want=%v, got=%v", c.expErr, err)
			}
			config, err := c.handler.Get(c.identityName)
			if err != nil {
				t.Errorf("want=nil, got=%v", err)
			}
			if !reflect.DeepEqual(config, c.config) {
				t.Errorf("want=%v, got=%v", c.config, config)
			}
		})
	}
}

func TestRemove(t *testing.T) {
	var (
		record1       = "record1"
		record2       = "record2"
		record2Config = &IdentityConfig{
			IdentityName: record2,
		}
		emptyHandler       = NewConfigCache()
		handlerWithRecord2 = NewConfigCache()
	)
	handlerWithRecord2.Update(record2, record2Config, NewConfigMapWriter(nil, logrus.WithFields(logrus.Fields{
		"type": "storeWorkloadData",
	})))
	cases := []struct {
		name         string
		identityName string
		handler      *cacheHandler
		expErr       error
	}{
		{
			name: "Given there is no record for '" + record1 + "', " +
				"When the Remove function is called, " +
				"It should return an error",
			identityName: record1,
			handler:      emptyHandler,
			expErr:       fmt.Errorf("config for %s does not exist", record1),
		},
		{
			name: "Given there is a record for '" + record2 + "', " +
				"When the Remove function is called, " +
				"It should remove the record",
			identityName: record2,
			handler:      handlerWithRecord2,
			expErr:       nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.handler.Remove(c.identityName)
			if !reflect.DeepEqual(err, c.expErr) {
				t.Errorf("want=%v, got=%v", c.expErr, err)
			}
		})
	}
}
