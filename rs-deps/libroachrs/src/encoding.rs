extern crate std;

const K_MVCC_VERSION_TIMESTAMP_SIZE: usize = 12;

use std::vec::Vec;

fn encode_uint32(b: &mut Vec<u8>, v: u32) {
    b.extend(&[
        (v>>24) as u8,
        (v>>16) as u8,
        (v>>8) as u8,
        v as u8])
}

fn encode_uint64(b: &mut Vec<u8>, v: u64) {
   b.extend(&[
        (v>>56) as u8,
        (v>>48) as u8,
        (v>>40) as u8,
        (v>>32) as u8,
        (v>>24) as u8,
        (v>>16) as u8,
        (v>>8) as u8,
        v as u8])
}

fn decode_uint32(b: &[u8], v: &mut u32) -> Option<usize> {
    let n = std::mem::size_of::<u32>();
    if b.len() < n {
        return None;
    }
    *v = (b[0] as u32) << 24 | (b[1] as u32) << 16 | (b[2] as u32) << 8 | (b[3] as u32);
    Some(n)
}

fn decode_uint64(b: &[u8], v: &mut u64) -> Option<usize> {
    let n = std::mem::size_of::<u64>();
    if b.len() < n {
        return None;
    }
    *v = (b[0] as u64) << 56 | (b[1] as u64) << 48 | (b[2] as u64) << 40 | (b[3] as u64) << 32 |
         (b[4] as u64) << 24 | (b[5] as u64) << 16 | (b[6] as u64) << 8 | (b[7] as u64);
    Some(n)
}

fn encode_timestamp(b: &mut Vec<u8>, wall_time: i64, logical: i32) {
  encode_uint64(b, wall_time as u64);
  if logical != 0 {
    encode_uint32(b, logical as u32);
  }
}

fn decode_timestamp(b: &[u8], wall_time: &mut i64, logical: &mut i32) -> Option<usize> {
    let w = &mut (0 as u64);
    let t = &mut (0 as u32);
    decode_uint64(b, w).and_then(|i| {
        *wall_time = *w as i64;
        match b.len() - i {
            0 => Some(i),
            _ => decode_uint32(&b[i..], t).and_then(|j| {
                Some(i+j)
            }),
        }
    }).and_then(|i| {
        *logical = *t as i32;
        Some(i)
    })
}

fn encode_key(key: &[u8], wall_time: i64, logical: i32) -> Vec<u8> {
    let has_ts = wall_time != 0 || logical != 0;
    let mut v = Vec::with_capacity(
        key.len() as usize + 1 + if has_ts {
            1 + K_MVCC_VERSION_TIMESTAMP_SIZE
        } else { 0 },
    );
    v.extend(key);
    if has_ts {
        v.push(0);
        encode_timestamp(&mut v, wall_time, logical);
    }
    let l = (v.len() - key.len()) as u8;
    v.push(l);
    v
}

fn split_key<'a>(b: &'a [u8]) -> Option<(&'a [u8], &'a [u8])> {
    let l = b.len();
    if l == 0 || b[l-1] as usize >= l {
        return None
    }
    Some(b.split_at(l - b[l-1] as usize - 1))
}

pub struct DBKey<'a> {
    key: &'a [u8],
    wall_time: i64,
    logical: i32,
}

fn decode_key<'a>(b: &'a [u8]) -> Option<DBKey<'a>> {
    split_key(b).and_then(|tuple|{
        let mut wall_time = 0 as i64;
        let mut logical = 0 as i32;
        if tuple.1.len() > 1 {
            if decode_timestamp(&tuple.1[1..], &mut wall_time, &mut logical).is_none() {
                return None
            }
        }
        Some(DBKey{
            key: tuple.0,
            wall_time: wall_time,
            logical: logical,
        })
    }).or(None)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn round_trip_key_with_ts() {
        let v = vec![1,2,3];
        let b = encode_key(&v, 12345, 6789);
        let db_key = decode_key(&b).expect("must be decodable");
        assert_eq!(db_key.key, v.as_slice());
        assert_eq!(db_key.wall_time, 12345);
        assert_eq!(db_key.logical, 6789);
    }

    #[test]
    fn round_trip_key_without_ts() {
        let v = vec![1,2,3];
        let b = encode_key(&v, 0, 0);
        assert_eq!(&b, &[1, 2, 3, 0]);
        assert!(split_key(&b).is_some());
        let db_key = decode_key(&b).expect("must be decodable");
        assert_eq!(db_key.key, v.as_slice());
        assert_eq!(db_key.wall_time, 0);
        assert_eq!(db_key.logical, 0);
    }

    #[test]
    fn round_trip_timestamp() {
        let b: &mut Vec<u8> = &mut vec![];
        encode_timestamp(b, 1234, 5678);
        encode_timestamp(b, 9012, 0);

        let wall_time = &mut (0 as i64);
        let logical = &mut (0 as i32);

        assert_eq!(decode_timestamp(b, wall_time, logical), Some(K_MVCC_VERSION_TIMESTAMP_SIZE));
        assert_eq!(*wall_time, 1234);
        assert_eq!(*logical, 5678);
        assert_eq!(decode_timestamp(&b[12..], wall_time, logical), Some(8));
        assert_eq!(*wall_time, 9012);
        assert_eq!(*logical, 0);

        // Slight divergence from the C++ code here in that wall_time and
        // logical are always modified there, but not necessarily here. It
        // doesn't matter (famous last words).
        assert_eq!(decode_timestamp(&b[0..11], wall_time, logical), None);
        assert_eq!(decode_timestamp(&b[0..2], wall_time, logical), None);
    }

    #[test]
    fn round_trip_uint() {
        let b: &mut Vec<u8> = &mut vec![];
        {
            encode_uint32(b, 12345);
            encode_uint64(b, 6789012345);
            encode_uint32(b, u32::max_value());
            encode_uint64(b, u64::max_value());
        }
        {
            let mut i = 0;
            let v32 = &mut (0 as u32);
            let v64 = &mut (0 as u64);

            i += decode_uint32(&b[i..], v32).unwrap();
            assert_eq!(*v32, 12345);

            i+= decode_uint64(&b[i..], v64).unwrap();
            assert_eq!(*v64, 6789012345);

            i += decode_uint32(&b[i..], v32).unwrap();
            assert_eq!(*v32, u32::max_value());

            i += decode_uint64(&b[i..], v64).unwrap();
            assert_eq!(*v64, u64::max_value());

            assert_eq!(i, 24);
            assert_eq!(b.len(), 24);
        }
    }

    #[test]
    fn short_slice_uint() {
        let b = vec![24, 85];
        let v32 = &mut (0 as u32);
        let v64 = &mut (0 as u64);

        assert_eq!(decode_uint64(&b, v64), None);
        assert_eq!(decode_uint32(&b, v32), None);
        assert_eq!(*v32, 0);
        assert_eq!(*v64, 0);
    }
}