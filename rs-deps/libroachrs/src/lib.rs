extern crate libc;
extern crate rocksdb;

use std::ffi::CStr;
use rocksdb::{DB, Options};

static STORAGEPATH: &'static str = "dummy-storage-location";

fn with_db<F, T>(f: F) -> T where F : Fn(DB) -> T {
    let db = DB::open_default(STORAGEPATH).unwrap();
    f(db)
}


#[no_mangle]
pub extern "C" fn destroy() {
    let opts = Options::default();
    assert!(DB::destroy(&opts, STORAGEPATH).is_ok());
}

#[no_mangle]
pub extern "C" fn put(k: *const libc::c_char, v: *const libc::c_char) {
    let key = unsafe { CStr::from_ptr(k).to_bytes().clone() };
    let value = unsafe { CStr::from_ptr(v).to_bytes().clone() };

    assert!(with_db(|db| db.put(key, value).is_ok()));
}

#[no_mangle]
pub extern "C" fn has(name: *const libc::c_char) -> libc::c_int {
    let key = unsafe { CStr::from_ptr(name).to_bytes().clone() };
    if _has(key) { 1 } else { 0 }
}

fn _has(key: &[u8]) -> bool {    
    match with_db(|db| db.get(key)) {
        Ok(Some(_)) => true,
        _ => false,
    }
}

#[cfg(test)]
mod tests {
    #[test]
    fn simple_example() {
        use *;
        assert!(with_db(|db| db.put(b"foo", b"bar").is_ok()));
        assert!(_has(b"foo"));
        destroy();
        assert!(!_has(b"foo"));
    }
}