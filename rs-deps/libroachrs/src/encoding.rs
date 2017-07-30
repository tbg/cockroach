extern crate std;

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

fn decode_uint32(b: &[u8], v: &mut u32) -> usize {
    let n = std::mem::size_of::<u32>();
    if b.len() < n {
        return 0;
    }
    *v = (b[0] as u32) << 24 | (b[1] as u32) << 16 | (b[2] as u32) << 8 | (b[3] as u32);
    n
}

fn decode_uint64(b: &[u8], v: &mut u64) -> usize {
    let n = std::mem::size_of::<u64>();
    if b.len() < n {
        return 0;
    }
    *v = (b[0] as u64) << 56 | (b[1] as u64) << 48 | (b[2] as u64) << 40 | (b[3] as u64) << 32 |
         (b[4] as u64) << 24 | (b[5] as u64) << 16 | (b[6] as u64) << 8 | (b[7] as u64);
    n
}

#[cfg(test)]
mod tests {
    use super::*;

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
            let v32: &mut u32 = &mut 0;
            let v64: &mut u64 = &mut 0;

            i += decode_uint32(&b[i..], v32);
            assert_eq!(*v32, 12345);

            i+= decode_uint64(&b[i..], v64);
            assert_eq!(*v64, 6789012345);
            
            i += decode_uint32(&b[i..], v32);
            assert_eq!(*v32, u32::max_value());

            i += decode_uint64(&b[i..], v64);
            assert_eq!(*v64, u64::max_value());

            assert_eq!(i, 24);
            assert_eq!(b.len(), 24);
        }
    }

    #[test]
    fn short_slice_uint() {
        let b = vec![24, 85];
        let v32: &mut u32 = &mut 0;
        let v64: &mut u64 = &mut 0;

        assert_eq!(decode_uint64(&b, v64), 0);
        assert_eq!(decode_uint32(&b, v32), 0);
        assert_eq!(*v32, 0);
        assert_eq!(*v64, 0);
    }
}