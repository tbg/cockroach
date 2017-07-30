extern crate std;
extern crate libc;

#[repr(C)]
pub struct DBStatus {
    pub data: *const libc::c_char,
    pub len: libc::c_int,
}

impl DBStatus {
    pub fn success() -> DBStatus {
        DBStatus {
            data: std::ptr::null(),
            len: 0,
        }
    }
}

#[repr(C)]
pub struct DBSlice {
    pub data: *const libc::c_char,
    pub len: libc::c_int,
}

#[repr(C)]
pub struct DBKey {
    key: DBSlice,
    wall_time: libc::int64_t,
    logical: libc::int32_t,
}