package colfer

import (
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/pascaldekloe/name"
)

// ECMAKeywords are the reserved tokens for ECMA Script.
// Some entries are redundant due to the use of a Go parser.
var eCMAKeywords = map[string]struct{}{
	"break": {}, "case": {}, "catch": {}, "class": {},
	"const": {}, "continue": {}, "debugger": {}, "default": {},
	"delete": {}, "do": {}, "else": {}, "enum": {},
	"export": {}, "extends": {}, "finally": {}, "for": {},
	"function": {}, "if": {}, "import": {}, "in": {},
	"instanceof": {}, "new": {}, "return": {}, "super": {},
	"switch": {}, "this": {}, "throw": {}, "try": {},
	"typeof": {}, "var": {}, "void": {}, "while": {},
	"with": {}, "yield": {},
}

// GenerateECMA writes the code into file "Colfer.js".
func GenerateECMA(basedir string, packages Packages) error {
	for _, p := range packages {
		p.NameNative = strings.Replace(p.Name, "/", "_", -1)
		if _, ok := eCMAKeywords[p.NameNative]; ok {
			p.NameNative += "_"
		}

		for _, t := range p.Structs {
			t.NameNative = name.CamelCase(t.Name, true)
			for _, f := range t.Fields {
				f.NameNative = name.CamelCase(f.Name, false)
				if _, ok := eCMAKeywords[f.NameNative]; ok {
					f.NameNative += "_"
				}
			}
		}
	}

	t := template.New("ecma-code")
	template.Must(t.Parse(ecmaCode))
	template.Must(t.New("marshal").Parse(ecmaMarshal))
	template.Must(t.New("unmarshal").Parse(ecmaUnmarshal))

	if err := os.MkdirAll(basedir, os.ModeDir|os.ModePerm); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(basedir, "Colfer.js"))
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, packages)
}

const ecmaCode = `/* eslint-disable no-redeclare */
// Code generated by colf(1); DO NOT EDIT.
{{- range .}}
// The compiler used schema file {{.SchemaFileList}} for package {{.Name}}.
{{- end}}
{{range .}}
{{.DocText "// "}}
class {{.Name}} {
	EOF = 'colfer: EOF';

	// The upper limit for serial byte sizes.
	colferSizeMax = {{.SizeMax}};
{{- if .HasList}}
	// The upper limit for the number of elements in a list.
	colferListMax = {{.ListMax}};
{{- end}}
{{range .Structs}}
	// Constructor.
{{.DocText "\t// "}}
	// When init is provided all enumerable properties are merged into the new object a.k.a. shallow cloning.
{{.NameNative}} = (init) => {
		return {
			{{template "marshal" .}}
			{{template "unmarshal" .}}
		};
	}
{{end}}
	// private section

	encodeVarint (bytes, i, x) {
		while (x > 127) {
			bytes[i++] = (x & 127) | 128;
			x /= 128;
		}
		bytes[i++] = x & 127;
		return i;
	}
{{if .HasTimestamp}}
	decodeInt64(data, i) {
		var v = 0, j = i + 7, m = 1;
		if (data[i] & 128) {
			// two's complement
			for (var carry = 1; j >= i; --j, m *= 256) {
				var b = (data[j] ^ 255) + carry;
				carry = b >> 8;
				v += (b & 255) * m;
			}
			v = -v;
		} else {
			for (; j >= i; --j, m *= 256)
				v += data[j] * m;
		}
		return v;
	}
{{end}}
	encodeUTF8(s) {
		var i = 0, bytes = new Uint8Array(s.length * 4);
		for (var ci = 0; ci != s.length; ci++) {
			var c = s.charCodeAt(ci);
			if (c < 128) {
				bytes[i++] = c;
				continue;
			}
			if (c < 2048) {
				bytes[i++] = c >> 6 | 192;
			} else {
				if (c > 0xd7ff && c < 0xdc00) {
					if (++ci >= s.length) {
						bytes[i++] = 63;
						continue;
					}
					var c2 = s.charCodeAt(ci);
					if (c2 < 0xdc00 || c2 > 0xdfff) {
						bytes[i++] = 63;
						--ci;
						continue;
					}
					c = 0x10000 + ((c & 0x03ff) << 10) + (c2 & 0x03ff);
					bytes[i++] = c >> 18 | 240;
					bytes[i++] = c >> 12 & 63 | 128;
				} else bytes[i++] = c >> 12 | 224;
				bytes[i++] = c >> 6 & 63 | 128;
			}
			bytes[i++] = c & 63 | 128;
		}
		return bytes.subarray(0, i);
	}

	decodeUTF8(bytes) {
		var i = 0, s = '';
		while (i < bytes.length) {
			var c = bytes[i++];
			if (c > 127) {
				if (c > 191 && c < 224) {
					c = (i >= bytes.length) ? 63 : (c & 31) << 6 | bytes[i++] & 63;
				} else if (c > 223 && c < 240) {
					c = (i + 1 >= bytes.length) ? 63 : (c & 15) << 12 | (bytes[i++] & 63) << 6 | bytes[i++] & 63;
				} else if (c > 239 && c < 248) {
					c = (i + 2 >= bytes.length) ? 63 : (c & 7) << 18 | (bytes[i++] & 63) << 12 | (bytes[i++] & 63) << 6 | bytes[i++] & 63;
				} else c = 63
			}

			if (c <= 0xffff) s += String.fromCharCode(c);
			else if (c > 0x10ffff) s += '?';
			else {
				c -= 0x10000;
				s += String.fromCharCode(c >> 10 | 0xd800)
				s += String.fromCharCode(c & 0x3FF | 0xdc00)
			}
		}
		return s;
	}
}

// NodeJS:
if (typeof exports !== 'undefined') exports.{{.NameNative}} = new {{.Name}}();
{{end}}`

const ecmaMarshal = `
	// Serializes the object into an Uint8Array.
{{- range .Fields}}{{if .TypeList}}{{if eq .Type "float32" "float64"}}{{else}}
	// All null entries in property {{.NameNative}} will be replaced with {{if eq .Type "text"}}an empty String{{else if eq .Type "binary"}}an empty Array{{else}}a new {{.TypeRef.Pkg.NameNative}}.{{.TypeRef.NameNative}}{{end}}.
{{- end}}{{end}}{{end}}
	marshal: (buf) => {
		if (! buf || !buf.length) buf = new Uint8Array(this.colferSizeMax);
		var i = 0;
		var view = new DataView(buf.buffer);

{{range .Fields}}{{if eq .Type "bool"}}
		if (init.{{.NameNative}})
			buf[i++] = {{.Index}};
{{else if eq .Type "uint8"}}
		if (init.{{.NameNative}}) {
			if (init.{{.NameNative}} > 255 || init.{{.NameNative}} < 0)
				throw new Error('colfer: {{.String}} out of reach: ' + init.{{.NameNative}});
			buf[i++] = {{.Index}};
			buf[i++] = init.{{.NameNative}};
		}
{{else if eq .Type "uint16"}}
		if (init.{{.NameNative}}) {
			if (init.{{.NameNative}} > 65535 || init.{{.NameNative}} < 0)
				throw new Error('colfer: {{.String}} out of reach: ' + init.{{.NameNative}});
			if (init.{{.NameNative}} < 256) {
				buf[i++] = {{.Index}} | 128;
				buf[i++] = init.{{.NameNative}};
			} else {
				buf[i++] = {{.Index}};
				buf[i++] = init.{{.NameNative}} >>> 0;
				buf[i++] = init.{{.NameNative}} & 255;
			}
		}
{{else if eq .Type "uint32"}}
		if (init.{{.NameNative}}) {
			if (init.{{.NameNative}} > 4294967295 || init.{{.NameNative}} < 0)
				throw new Error('colfer: {{.String}} out of reach: ' + init.{{.NameNative}});
			if (init.{{.NameNative}} < 0x200000) {
				buf[i++] = {{.Index}};
				i = this.encodeVarint(buf, i, init.{{.NameNative}});
			} else {
				buf[i++] = {{.Index}} | 128;
				view.setUint32(i, init.{{.NameNative}});
				i += 4;
			}
		}
{{else if eq .Type "uint64"}}
		if (init.{{.NameNative}}) {
			if (init.{{.NameNative}} < 0)
				throw new Error('colfer: {{.String}} out of reach: ' + init.{{.NameNative}});
			if (init.{{.NameNative}} > Number.MAX_SAFE_INTEGER)
				throw new Error('colfer: {{.String}} exceeds Number.MAX_SAFE_INTEGER');
			if (init.{{.NameNative}} < 0x2000000000000) {
				buf[i++] = {{.Index}};
				i = this.encodeVarint(buf, i, init.{{.NameNative}});
			} else {
				buf[i++] = {{.Index}} | 128;
				view.setUint32(i, init.{{.NameNative}} / 0x100000000);
				i += 4;
				view.setUint32(i, init.{{.NameNative}} % 0x100000000);
				i += 4;
			}
		}
{{else if eq .Type "int32"}}
		if (init.{{.NameNative}}) {
			if (init.{{.NameNative}} < 0) {
				buf[i++] = {{.Index}} | 128;
				if (init.{{.NameNative}} < -2147483648)
					throw new Error('colfer: {{.String}} exceeds 32-bit range');
				i = this.encodeVarint(buf, i, -init.{{.NameNative}});
			} else {
				buf[i++] = {{.Index}}; 
				if (init.{{.NameNative}} > 2147483647)
					throw new Error('colfer: {{.String}} exceeds 32-bit range');
				i = this.encodeVarint(buf, i, init.{{.NameNative}});
			}
		}
{{else if eq .Type "int64"}}
		if (init.{{.NameNative}}) {
			if (init.{{.NameNative}} < 0) {
				buf[i++] = {{.Index}} | 128;
				if (init.{{.NameNative}} < Number.MIN_SAFE_INTEGER)
					throw new Error('colfer: {{.String}} exceeds Number.MIN_SAFE_INTEGER');
				i = this.encodeVarint(buf, i, -init.{{.NameNative}});
			} else {
				buf[i++] = {{.Index}}; 
				if (init.{{.NameNative}} > Number.MAX_SAFE_INTEGER)
					throw new Error('colfer: {{.String}} exceeds Number.MAX_SAFE_INTEGER');
				i = this.encodeVarint(buf, i, init.{{.NameNative}});
			}
		}
{{else if eq .Type "float32"}}
 {{- if .TypeList}}
		if (init.{{.NameNative}} && init.{{.NameNative}}.length) {
			var a = init.{{.NameNative}};
			if (a.length > this.colferListMax)
				throw new Error('colfer: {{.String}} exceeds colferListMax');
			buf[i++] = {{.Index}};
			i = this.encodeVarint(buf, i, a.length);
			a.forEach(function(f, fi) {
				if (f > 3.4028234663852886E38 || f < -3.4028234663852886E38)
					throw new Error('colfer: {{.String}}[' + fi + '] exceeds 32-bit range');
				view.setFloat32(i, f);
				i += 4;
			});
		}
 {{- else}}
		if (init.{{.NameNative}}) {
			if (init.{{.NameNative}} > 3.4028234663852886E38 || init.{{.NameNative}} < -3.4028234663852886E38)
				throw new Error('colfer: {{.String}} exceeds 32-bit range');
			buf[i++] = {{.Index}};
			view.setFloat32(i, init.{{.NameNative}});
			i += 4;
		} else if (Number.isNaN(init.{{.NameNative}})) {
			buf.set([{{.Index}}, 0x7f, 0xc0, 0, 0], i);
			i += 5;
		}
 {{- end}}
{{else if eq .Type "float64"}}
 {{- if .TypeList}}
		if (init.{{.NameNative}} && init.{{.NameNative}}.length) {
			var a = init.{{.NameNative}};
			if (a.length > this.colferListMax)
				throw new Error('colfer: {{.String}} exceeds colferListMax');
			buf[i++] = {{.Index}};
			i = this.encodeVarint(buf, i, a.length);
			a.forEach(function(f) {
				view.setFloat64(i, f);
				i += 8;
			});
		}
 {{- else}}
		if (init.{{.NameNative}}) {
			buf[i++] = {{.Index}};
			view.setFloat64(i, init.{{.NameNative}});
			i += 8;
		} else if (Number.isNaN(init.{{.NameNative}})) {
			buf.set([{{.Index}}, 0x7f, 0xf8, 0, 0, 0, 0, 0, 0], i);
			i += 9;
		}
 {{- end}}
{{else if eq .Type "timestamp"}}
		if ((init.{{.NameNative}} && init.{{.NameNative}}.getTime()) || init.{{.NameNative}}_ns) {
			var ms = init.{{.NameNative}} ? init.{{.NameNative}}.getTime() : 0;
			var s = ms / 1E3;

			var ns = init.{{.NameNative}}_ns || 0;
			if (ns < 0 || ns >= 1E6)
				throw new Error('colfer: {{.String}} ns not in range (0, 1ms>');
			var msf = ms % 1E3;
			if (ms < 0 && msf) {
				s--
				msf = 1E3 + msf;
			}
			ns += msf * 1E6;

			if (s > 0xffffffff || s < 0) {
				buf[i++] = {{.Index}} | 128;
				if (s > 0) {
					view.setUint32(i, s / 0x100000000);
					view.setUint32(i + 4, s);
				} else {
					s = -s;
					view.setUint32(i, s / 0x100000000);
					view.setUint32(i + 4, s);
					var carry = 1;
					for (var j = i + 7; j >= i; j--) {
						var b = (buf[j] ^ 255) + carry;
						buf[j] = b & 255;
						carry = b >> 8;
					}
				}
				view.setUint32(i + 8, ns);
				i += 12;
			} else {
				buf[i++] = {{.Index}};
				view.setUint32(i, s);
				i += 4;
				view.setUint32(i, ns);
				i += 4;
			}
		}
{{else if eq .Type "text"}}
 {{- if .TypeList}}
		if (init.{{.NameNative}} && init.{{.NameNative}}.length) {
			var a = init.{{.NameNative}};
			if (a.length > this.colferListMax)
				throw new Error('colfer: {{.String}} exceeds colferListMax');
			buf[i++] = {{.Index}};
			i = this.encodeVarint(buf, i, a.length);

			a.forEach((s, si) => {
				if (s == null) {
					s = "";
					a[si] = s;
				}
				var utf8 = this.encodeUTF8(s);
				i = this.encodeVarint(buf, i, utf8.length);
				buf.set(utf8, i);
				i += utf8.length;
			});
		}
 {{- else}}
		if (init.{{.NameNative}}) {
			buf[i++] = {{.Index}};
			var utf8 = this.encodeUTF8(init.{{.NameNative}});
			i = this.encodeVarint(buf, i, utf8.length);
			buf.set(utf8, i);
			i += utf8.length;
		}
 {{- end}}
{{else if eq .Type "binary"}}
 {{- if .TypeList}}
		if (init.{{.NameNative}} && init.{{.NameNative}}.length) {
			var a = init.{{.NameNative}};
			if (a.length > this.colferListMax)
				throw new Error('colfer: {{.String}} exceeds colferListMax');
			buf[i++] = {{.Index}};
			i = this.encodeVarint(buf, i, a.length);
			a.forEach((b, bi) => {
				if (b == null) {
					b = "";
					a[bi] = b;
				}
				i = this.encodeVarint(buf, i, b.length);
				buf.set(b, i);
				i += b.length;
			});
		}
 {{- else}}
		if (init.{{.NameNative}} && init.{{.NameNative}}.length) {
			buf[i++] = {{.Index}};
			var b = init.{{.NameNative}};
			i = this.encodeVarint(buf, i, b.length);
			buf.set(b, i);
			i += b.length;
		}
 {{- end}}
{{else if .TypeList}}
		if (init.{{.NameNative}} && init.{{.NameNative}}.length) {
			var a = init.{{.NameNative}};
			if (a.length > this.colferListMax)
				throw new Error('colfer: {{.String}} exceeds colferListMax');
			buf[i++] = {{.Index}};
			i = this.encodeVarint(buf, i, a.length);
			a.forEach(function(v, vi) {
				if (v == null) {
					v = new {{.TypeRef.Pkg.NameNative}}.{{.TypeRef.NameNative}}();
					a[vi] = v;
				}
				var b = v.marshal();
				buf.set(b, i);
				i += b.length;
			});
		}
{{else}}
		if (init.{{.NameNative}}) {
			buf[i++] = {{.Index}};
			var b = init.{{.NameNative}}.marshal();
			buf.set(b, i);
			i += b.length;
		}
{{end}}{{end}}

		buf[i++] = 127;
		if (i >= this.colferSizeMax)
			throw new Error('colfer: {{.String}} serial size ' + i + ' exceeds ' + this.colferSizeMax + ' bytes');
		return buf.subarray(0, i);
	},`

const ecmaUnmarshal = `
	// Deserializes the object from an Uint8Array and returns the number of bytes read.
	unmarshal: (data) => {
		if (!data || ! data.length) throw new Error(this.EOF);
		var header = data[0];
		var i = 1;
		var readHeader = function() {
			if (i >= data.length) throw new Error(this.EOF);
			header = data[i++];
		}

		var view = new DataView(data.buffer, data.byteOffset, data.byteLength);

		var readVarint = function() {
			var pos = 0, result = 0;
			while (pos != 8) {
				var c = data[i+pos];
				result += (c & 127) * Math.pow(128, pos);
				++pos;
				if (c < 128) {
					i += pos;
					if (result > Number.MAX_SAFE_INTEGER) break;
					return result;
				}
				if (pos == data.length) throw new Error(this.EOF);
			}
			return -1;
		}
{{range .Fields}}{{if eq .Type "bool"}}
		if (header == {{.Index}}) {
			init.{{.NameNative}} = true;
			readHeader();
		}
{{else if eq .Type "uint8"}}
		if (header == {{.Index}}) {
			if (i + 1 >= data.length) throw new Error(this.EOF);
			init.{{.NameNative}} = data[i++];
			header = data[i++];
		}
{{else if eq .Type "uint16"}}
		if (header == {{.Index}}) {
			if (i + 2 >= data.length) throw new Error(this.EOF);
			init.{{.NameNative}} = (data[i++] << 8) | data[i++];
			header = data[i++];
		} else if (header == ({{.Index}} | 128)) {
			if (i + 1 >= data.length) throw new Error(this.EOF);
			init.{{.NameNative}} = data[i++];
			header = data[i++];
		}
{{else if eq .Type "uint32"}}
		if (header == {{.Index}}) {
			var x = readVarint();
			if (x < 0) throw new Error('colfer: {{.String}} exceeds Number.MAX_SAFE_INTEGER');
			init.{{.NameNative}} = x;
			readHeader();
		} else if (header == ({{.Index}} | 128)) {
			if (i + 4 > data.length) throw new Error(this.EOF);
			init.{{.NameNative}} = view.getUint32(i);
			i += 4;
			readHeader();
		}
{{else if eq .Type "uint64"}}
		if (header == {{.Index}}) {
			var x = readVarint();
			if (x < 0) throw new Error('colfer: {{.String}} exceeds Number.MAX_SAFE_INTEGER');
			init.{{.NameNative}} = x;
			readHeader();
		} else if (header == ({{.Index}} | 128)) {
			if (i + 8 > data.length) throw new Error(this.EOF);
			var x = view.getUint32(i) * 0x100000000;
			x += view.getUint32(i + 4);
			if (x > Number.MAX_SAFE_INTEGER)
				throw new Error('colfer: {{.String}} exceeds Number.MAX_SAFE_INTEGER');
			init.{{.NameNative}} = x;
			i += 8;
			readHeader();
		}
{{else if eq .Type "int32"}}
		if (header == {{.Index}}) {
			var x = readVarint();
			if (x < 0) throw new Error('colfer: {{.String}} exceeds Number.MAX_SAFE_INTEGER');
			init.{{.NameNative}} = x;
			readHeader();
		} else if (header == ({{.Index}} | 128)) {
			var x = readVarint();
			if (x < 0) throw new Error('colfer: {{.String}} exceeds Number.MAX_SAFE_INTEGER');
			init.{{.NameNative}} = -1 * x;
			readHeader();
		}
{{else if eq .Type "int64"}}
		if (header == {{.Index}}) {
			var x = readVarint();
			if (x < 0) throw new Error('colfer: {{.String}} exceeds Number.MAX_SAFE_INTEGER');
			init.{{.NameNative}} = x;
			readHeader();
		} else if (header == ({{.Index}} | 128)) {
			var x = readVarint();
			if (x < 0) throw new Error('colfer: {{.String}} exceeds Number.MAX_SAFE_INTEGER');
			init.{{.NameNative}} = -1 * x;
			readHeader();
		}
{{else if eq .Type "float32"}}
		if (header == {{.Index}}) {
 {{- if .TypeList}}
			var l = readVarint();
			if (l < 0) throw new Error('colfer: {{.String}} length exceeds Number.MAX_SAFE_INTEGER');
			if (l > this.colferListMax)
				throw new Error('colfer: {{.String}} length ' + l + ' exceeds ' + this.colferListMax + ' elements');
			if (i + l * 4 > data.length) throw new Error(this.EOF);

			init.{{.NameNative}} = new Float32Array(l);
			for (var n = 0; n < l; ++n) {
				init.{{.NameNative}}[n] = view.getFloat32(i);
				i += 4;
			}
 {{- else}}
			if (i + 4 > data.length) throw new Error(this.EOF);
			init.{{.NameNative}} = view.getFloat32(i);
			i += 4;
 {{- end}}
			readHeader();
		}
{{else if eq .Type "float64"}}
		if (header == {{.Index}}) {
 {{- if .TypeList}}
			var l = readVarint();
			if (l < 0 || l > this.colferListMax)
				throw new Error('colfer: {{.String}} length ' + l + ' exceeds ' + this.colferListMax + ' elements');
			if (i + l * 8 > data.length) throw new Error(this.EOF);

			init.{{.NameNative}} = new Float64Array(l);
			for (var n = 0; n < l; ++n) {
				init.{{.NameNative}}[n] = view.getFloat64(i);
				i += 8;
			}
 {{- else}}
			if (i + 8 > data.length) throw new Error(this.EOF);
			init.{{.NameNative}} = view.getFloat64(i);
			i += 8;
 {{- end}}
			readHeader();
		}
{{else if eq .Type "timestamp"}}
		if (header == {{.Index}}) {
			if (i + 8 > data.length) throw new Error(this.EOF);

			var ms = view.getUint32(i) * 1E3;
			var ns = view.getUint32(i + 4);
			ms += Math.floor(ns / 1E6);
			init.{{.NameNative}} = new Date(ms);
			init.{{.NameNative}}_ns = ns % 1E6;

			i += 8;
			readHeader();
		} else if (header == ({{.Index}} | 128)) {
			if (i + 12 > data.length) throw new Error(this.EOF);

			var ms = decodeInt64(data, i) * 1E3;
			var ns = view.getUint32(i + 8);
			ms += Math.floor(ns / 1E6);
			if (ms < -864E13 || ms > 864E13)
				throw new Error('colfer: {{.String}} exceeds ECMA Date range');
			init.{{.NameNative}} = new Date(ms);
			init.{{.NameNative}}_ns = ns % 1E6;

			i += 12;
			readHeader();
		}
{{else if eq .Type "text"}}
		if (header == {{.Index}}) {
 {{- if .TypeList}}
			var l = readVarint();
			if (l < 0 || l > this.colferListMax)
				throw new Error('colfer: {{.String}} length ' + l + ' exceeds ' + this.colferListMax + ' elements');

			init.{{.NameNative}} = new Array(l);
			for (var n = 0; n < l; ++n) {
				var size = readVarint();
				if (size < 0 || size > this.colferSizeMax)
					throw new Error('colfer: {{.String}}[' + init.{{.NameNative}}.length + '] size ' + size + ' exceeds ' + this.colferSizeMax + ' bytes');

				var start = i;
				i += size;
				if (i > data.length) throw new Error(this.EOF);
				init.{{.NameNative}}[n] = this.decodeUTF8(data.subarray(start, i));
			}
 {{- else}}
			var size = readVarint();
			if (size < 0 || size > this.colferSizeMax)
				throw new Error('colfer: {{.String}} size ' + size + ' exceeds ' + this.colferSizeMax + ' bytes');

			var start = i;
			i += size;
			if (i > data.length) throw new Error(this.EOF);
			init.{{.NameNative}} = this.decodeUTF8(data.subarray(start, i));
 {{- end}}
			readHeader();
		}
{{else if eq .Type "binary"}}
		if (header == {{.Index}}) {
 {{- if .TypeList}}
			var l = readVarint();
			if (l < 0 || l > this.colferListMax)
				throw new Error('colfer: {{.String}} length ' + l + ' exceeds ' + this.colferListMax + ' elements');

			init.{{.NameNative}} = new Array(l);
			for (var n = 0; n < l; ++n) {
				var size = readVarint();
				if (size < 0 || size > this.colferSizeMax)
					throw new Error('colfer: {{.String}}[' + init.{{.NameNative}}.length + '] size ' + size + ' exceeds ' + this.colferSizeMax + ' bytes');

				var start = i;
				i += size;
				if (i > data.length) throw new Error(this.EOF);
				init.{{.NameNative}}[n] = data.slice(start, i);
			}
 {{- else}}
			var size = readVarint();
			if (size < 0 || size > this.colferSizeMax)
				throw new Error('colfer: {{.String}} size ' + size + ' exceeds ' + this.colferSizeMax + ' bytes');

			var start = i;
			i += size;
			if (i > data.length) throw new Error(this.EOF);
			init.{{.NameNative}} = data.slice(start, i);
 {{- end}}
			readHeader();
		}
{{else if .TypeList}}
		if (header == {{.Index}}) {
			var l = readVarint();
			if (l < 0 || l > this.colferListMax)
				throw new Error('colfer: {{.String}} length ' + l + ' exceeds ' + this.colferListMax + ' elements');

			for (var n = 0; n < l; ++n) {
				var o = new {{.TypeRef.Pkg.NameNative}}.{{.TypeRef.NameNative}}();
				i += o.unmarshal(data.subarray(i));
				init.{{.NameNative}}[n] = o;
			}
			readHeader();
		}
{{else}}
		if (header == {{.Index}}) {
			var o = new {{.TypeRef.Pkg.NameNative}}.{{.TypeRef.NameNative}}();
			i += o.unmarshal(data.subarray(i));
			init.{{.NameNative}} = o;
			readHeader();
		}
{{end}}{{end}}
		if (header != 127) throw new Error('colfer: unknown header at byte ' + (i - 1));
		if (i > this.colferSizeMax)
			throw new Error('colfer: {{.String}} serial size ' + size + ' exceeds ' + this.colferSizeMax + ' bytes');
		return init;
	}`
