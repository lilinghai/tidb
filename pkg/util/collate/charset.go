// Copyright 2021 PingCAP, Inc.
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

package collate

import "github.com/pingcap/tidb/pkg/parser/charset"

// switchDefaultCollation switch the default collation for charset according to the new collation config.
func switchDefaultCollation(flag bool) {
	if flag {
		charset.CharacterSetInfos[charset.CharsetGBK].DefaultCollation = charset.CollationGBKChineseCI
		charset.CharacterSetInfos[charset.CharsetGB18030].DefaultCollation = charset.CollationGB18030ChineseCI
	} else {
		charset.CharacterSetInfos[charset.CharsetGBK].DefaultCollation = charset.CollationGBKBin
		charset.CharacterSetInfos[charset.CharsetGB18030].DefaultCollation = charset.CollationGB18030Bin
	}
	charset.CharacterSetInfos[charset.CharsetGBK].Collations[charset.CollationGBKBin].IsDefault = !flag
	charset.CharacterSetInfos[charset.CharsetGBK].Collations[charset.CollationGBKChineseCI].IsDefault = flag
	charset.CharacterSetInfos[charset.CharsetGB18030].Collations[charset.CollationGB18030Bin].IsDefault = !flag
	charset.CharacterSetInfos[charset.CharsetGB18030].Collations[charset.CollationGB18030ChineseCI].IsDefault = flag
}
