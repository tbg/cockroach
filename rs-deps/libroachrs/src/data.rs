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