extern crate std;
extern crate libc;

#[repr(C)]
pub struct DBStatus {
    pub data: *const libc::c_char,
    pub len: libc::c_int,
}

pub unsafe fn for_go(v: Vec<u8>) -> (*const libc::c_char, libc::c_int) {
    let b = v.into_boxed_slice();
    let l = b.len();
    let data = Box::into_raw(b);
    (data as *const libc::c_char, l as libc::c_int)
}

#[no_mangle]
pub unsafe fn from_go(ptr: *mut libc::c_char, len: libc::c_int) {
    Vec::from_raw_parts(ptr, len as usize, len as usize);
    // free() happens here.
}

impl DBStatus {
    pub fn success() -> DBStatus {
        DBStatus {
            data: std::ptr::null(),
            len: 0,
        }
    }
    pub fn from_str(s: String) -> DBStatus {
        let (data, len) = unsafe { for_go(s.into_bytes()) };
        DBStatus {
            data: data,
            len: len,
        }
    }
}

#[repr(C)]
pub struct DBSlice {
    pub data: *const libc::c_char,
    pub len: libc::c_int,
}

#[repr(C)]
pub struct DBString {
    pub data: *const libc::c_char,
    pub len: libc::c_int,
}

#[repr(C)]
pub struct DBKey {
    pub key: DBSlice,
    pub wall_time: libc::int64_t,
    pub logical: libc::int32_t,
}