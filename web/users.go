// Copyright 2020 The Prometheus Authors
// This code is partly borrowed from Caddy:
//    Copyright 2015 Matthew Holt and The Caddy Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package web

import (
	"encoding/hex"
	"net/http"

	"github.com/go-kit/kit/log"
	"golang.org/x/crypto/bcrypt"
)

func validateUsers(configPath string) error {
	c, err := getConfig(configPath)
	if err != nil {
		return err
	}

	for _, p := range c.Users {
		_, err = bcrypt.Cost([]byte(p))
		if err != nil {
			return err
		}
	}

	return nil
}

type userAuthRoundtrip struct {
	tlsConfigPath string
	handler       http.Handler
	logger        log.Logger
	cache         *cache
}

func (u *userAuthRoundtrip) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := getConfig(u.tlsConfigPath)
	if err != nil {
		u.logger.Log("msg", "Unable to parse configuration", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if len(c.Users) == 0 {
		u.handler.ServeHTTP(w, r)
		return
	}

	user, pass, auth := r.BasicAuth()
	if auth {
		if hashedPassword, ok := c.Users[user]; ok {
			cacheKey := hex.EncodeToString(append(append([]byte(user), []byte(hashedPassword)...), []byte(pass)...))
			u.cache.mtx.Lock()
			if valid, ok := u.cache.cache[cacheKey]; valid && ok {
				u.cache.mtx.Unlock()
				u.handler.ServeHTTP(w, r)
				return
			}
			u.cache.makeRoom()
			u.cache.mtx.Unlock()
			err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(pass))
			authOk := err == nil
			u.cache.mtx.Lock()
			u.cache.cache[cacheKey] = authOk
			u.cache.mtx.Unlock()
			if authOk {
				u.handler.ServeHTTP(w, r)
				return
			}
		}
	}

	w.Header().Set("WWW-Authenticate", "Basic")
	http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
}
