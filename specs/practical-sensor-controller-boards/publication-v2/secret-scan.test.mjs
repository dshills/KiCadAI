import assert from 'node:assert/strict';
import test from 'node:test';
import {hasKeyShapedString} from './evidence-authentication.mjs';
test('chunked scan handles clean large buffers',()=>assert.equal(hasKeyShapedString(Buffer.alloc(3*1024*1024,120)),false));
test('chunk boundary cannot hide a synthetic key-shaped string',()=>{const b=Buffer.alloc(2*1024*1024,46);b.write('sk-'+ 'x'.repeat(30),1024*1024-9);assert.equal(hasKeyShapedString(b),true);});
test('short ordinary fragments are not treated as credentials',()=>assert.equal(hasKeyShapedString(Buffer.from('short sk-x fragment')),false));
