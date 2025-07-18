// Copyright 2022-2023 EMQ Technologies Co., Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package definition

import "time"

type Database interface {
	Connect() error
	Disconnect() error
}

type Config struct {
	Type         string
	ExtStateType string
	Redis        RedisConfig
	Sqlite       SqliteConfig
	Fdb          FdbConfig
	Pebble       PebbleConfig
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	Timeout  time.Duration
}

type SqliteConfig struct {
	Path string
	Name string
}

type FdbConfig struct {
	Path       string
	APIVersion int
	Timeout    int64
}

type PebbleConfig struct {
	Path string
	Name string
}
