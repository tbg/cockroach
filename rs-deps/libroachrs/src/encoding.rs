extern crate std;

const K_MVCC_VERSION_TIMESTAMP_SIZE: usize = 12;

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

#[cfg(test)]
mod tests {
    use super::*;

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