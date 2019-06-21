/*
 * Copyright 2019 IBM Corporation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package util

import "reflect"

// SetField sets a field in a struct using reflect
func SetField(obj interface{}, field string, value interface{}) {
	v := reflect.ValueOf(obj)
	if v.Type().Kind() == reflect.Ptr {
		v = v.Elem()
	}
	v = v.FieldByName(field)
	if v.IsValid() && v.CanSet() {
		v.Set(reflect.ValueOf(value))
	}
}

// SetField sets a field in a struct to type specific zero using reflect
func SetFieldToZero(obj interface{}, field string) {
	v := reflect.ValueOf(obj)
	if v.Type().Kind() == reflect.Ptr {
		v = v.Elem()
	}
	v = v.FieldByName(field)
	if v.Type().Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.IsValid() && v.CanSet() {
		v.Set(reflect.Zero(v.Type()))
	}
}

// GetField gets a field in a struct using reflect
func GetField(obj interface{}, field string) interface{} {
	v := reflect.ValueOf(obj)
	if v.Type().Kind() == reflect.Ptr {
		v = v.Elem()
	}
	f := v.FieldByName(field)
	if f.IsValid() {
		return f.Interface()
	}
	return nil
}
