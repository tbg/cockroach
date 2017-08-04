extern crate libc;
extern crate rocksdb;

mod encoding;
mod data;
mod tscache;

use std::ffi::CStr;

use data::*;

pub use data::from_go;

// See https://github.com/shepmaster/rust-ffi-omnibus/blob/master/examples/objects/src/lib.rs
// for some inspiration on calling into Rust.

static STORAGEPATH: &'static str = "dummy-storage-location";



#[repr(C)]
pub struct DBEngine {
    db: rocksdb::DB,
}

impl DBEngine {
    fn new(dir: &std::path::Path) -> Result<DBEngine, rocksdb::Error> {
        rocksdb::DB::open_default(dir).and_then(|db| Ok(DBEngine{db}))
    }
}

#[no_mangle]
pub extern "C" fn dbengine_open(ptr: *mut *mut DBEngine, dir: *const libc::c_char) -> DBStatus {
    unsafe {
        let db = DBEngine::new(std::path::Path::new(CStr::from_ptr(dir).to_str().unwrap())).unwrap();
        *ptr = Box::into_raw(Box::new(db));
    }
    DBStatus::success()
}

#[no_mangle]
pub extern "C" fn dbengine_close(ptr: *mut DBEngine) -> DBStatus {
    unsafe { Box::from_raw(ptr); } // frees when the Box goes out of scope
    DBStatus::success()
}

#[no_mangle]
pub extern "C" fn dbengine_put(dbe: *mut DBEngine, k: *const libc::c_char, v: *const libc::c_char) {
    let k = unsafe { CStr::from_ptr(k).to_bytes() };
    let v = unsafe { CStr::from_ptr(v).to_bytes() };
    unsafe {
        assert!((*dbe).db.put(k,v).is_ok())
    }
}

#[no_mangle]
pub extern "C" fn dbengine_get(dbe: *mut DBEngine, k: DBKey, v: *mut DBString) -> DBStatus {
    let k = unsafe { std::slice::from_raw_parts(k.key.data as *const u8, k.key.len as usize) };
    let res = unsafe { (*dbe).db.get(k) };
    match res {
        Err(err) => DBStatus::from_str(err.to_string()),
        Ok(None) => {
            unsafe {
                *v = DBString {
                    data: std::ptr::null(),
                    len: 0,
                }
            };
            DBStatus::success()
        }
        Ok(Some(value)) => {
            let sl: &[u8] = &value;
            // TODO(tschottdorf): this copy can likely be avoided by using
            // db.raw_iterator() instead, but for now let's play it safe.
            // The C++ code also eats a similar copy.
            let s = Vec::<u8>::from(sl);
            let (data, len) = unsafe { data::for_go(s) };
            unsafe {
                *v = DBString {
                    data: data,
                    len: len,
                }
            };
            DBStatus::success()
        }
    }
}

fn with_db<F, T>(f: F) -> T where F : Fn(rocksdb::DB) -> T {
    let db = DBEngine::new(std::path::Path::new(STORAGEPATH)).unwrap();
    f(db.db)
}


#[no_mangle]
pub extern "C" fn destroy() {
    let opts = rocksdb::Options::default();
    assert!(rocksdb::DB::destroy(&opts, STORAGEPATH).is_ok());
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
