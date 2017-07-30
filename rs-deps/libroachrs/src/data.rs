extern crate std;
extern crate libc;

#[repr(C)]
pub struct DBStatus {
    data: *const libc::c_char,
    len: libc::c_int,
}

impl DBStatus {
    pub fn success() -> DBStatus {
        DBStatus {
            data: std::ptr::null(),
            len: 0,
        }
    }
}