// Copyright 2017 The Cockroach Authors.
//
// Licensed as a CockroachDB Enterprise file under the Cockroach Community
// License (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     https://github.com/cockroachdb/cockroach/blob/master/licenses/CCL.txt

#include "db.h"
#include <iostream>
#include <libroachccl.h>
#include <memory>
#include <rocksdb/comparator.h>
#include <rocksdb/iterator.h>
#include <rocksdb/utilities/write_batch_with_index.h>
#include <rocksdb/write_batch.h>
#include "../batch.h"
#include "../comparator.h"
#include "../encoding.h"
#include "../env_manager.h"
#include "../options.h"
#include "../rocksdbutils/env_encryption.h"
#include "../status.h"
#include "ccl/baseccl/encryption_options.pb.h"
#include "ccl/storageccl/engineccl/enginepbccl/stats.pb.h"
#include "crypto_utils.h"
#include "ctr_stream.h"
#include "key_manager.h"

using namespace cockroach;

namespace cockroach {

class CCLEnvStatsHandler : public EnvStatsHandler {
 public:
  explicit CCLEnvStatsHandler(DataKeyManager* data_key_manager)
      : data_key_manager_(data_key_manager) {}
  virtual ~CCLEnvStatsHandler() {}

  virtual rocksdb::Status GetEncryptionStats(std::string* serialized_stats) override {
    if (data_key_manager_ == nullptr) {
      return rocksdb::Status::OK();
    }

    enginepbccl::EncryptionStatus enc_status;

    // GetActiveStoreKeyInfo returns a unique_ptr containing a copy. Transfer ownership from the
    // unique_ptr to the proto. set_allocated_active_store deletes the existing field, if any.
    enc_status.set_allocated_active_store_key(data_key_manager_->GetActiveStoreKeyInfo().release());

    // CurrentKeyInfo returns a unique_ptr containing a copy. Transfer ownership from the
    // unique_ptr to the proto. set_allocated_active_store deletes the existing field, if any.
    enc_status.set_allocated_active_data_key(data_key_manager_->CurrentKeyInfo().release());

    if (!enc_status.SerializeToString(serialized_stats)) {
      return rocksdb::Status::InvalidArgument("failed to serialize encryption status");
    }
    return rocksdb::Status::OK();
  }

  virtual rocksdb::Status GetEncryptionRegistry(std::string* serialized_registry) override {
    if (data_key_manager_ == nullptr) {
      return rocksdb::Status::OK();
    }

    auto key_registry = data_key_manager_->GetScrubbedRegistry();
    if (key_registry == nullptr) {
      return rocksdb::Status::OK();
    }

    if (!key_registry->SerializeToString(serialized_registry)) {
      return rocksdb::Status::InvalidArgument("failed to serialize data keys registry");
    }

    return rocksdb::Status::OK();
  }

  virtual std::string GetActiveDataKeyID() override {
    // Look up the current data key.
    if (data_key_manager_ == nullptr) {
      // No data key manager: plaintext.
      return kPlainKeyID;
    }

    auto active_key_info = data_key_manager_->CurrentKeyInfo();
    if (active_key_info == nullptr) {
      // Plaintext.
      return kPlainKeyID;
    }
    return active_key_info->key_id();
  }

  virtual rocksdb::Status GetFileEntryKeyID(const enginepb::FileEntry* entry,
                                            std::string* id) override {
    if (entry == nullptr) {
      // No file entry: file written in plaintext before the file registry was used.
      *id = kPlainKeyID;
      return rocksdb::Status::OK();
    }

    enginepbccl::EncryptionSettings enc_settings;
    if (!enc_settings.ParseFromString(entry->encryption_settings())) {
      return rocksdb::Status::InvalidArgument("failed to parse encryption settings");
    }

    if (enc_settings.encryption_type() == enginepbccl::Plaintext) {
      *id = kPlainKeyID;
    } else {
      *id = enc_settings.key_id();
    }

    return rocksdb::Status::OK();
  }

 private:
  // The DataKeyManager is needed to get key information but is not owned by the StatsHandler.
  DataKeyManager* data_key_manager_;
};

// DBOpenHookCCL parses the extra_options field of DBOptions and initializes
// encryption objects if needed.
rocksdb::Status DBOpenHookCCL(std::shared_ptr<rocksdb::Logger> info_log, const std::string& db_dir,
                              const DBOptions db_opts, EnvManager* env_mgr) {
  UsesAESNI();
  return rocksdb::Status::OK();
}

}  // namespace cockroach

void* DBOpenHookCCL = (void*)cockroach::DBOpenHookCCL;

DBStatus DBBatchReprVerify(DBSlice repr, DBKey start, DBKey end, int64_t now_nanos,
                           MVCCStatsResult* stats) {
  // TODO(dan): Inserting into a batch just to iterate over it is unfortunate.
  // Consider replacing this with WriteBatch's Iterate/Handler mechanism and
  // computing MVCC stats on the post-ApplyBatchRepr engine. splitTrigger does
  // the latter and it's a headache for propEvalKV, so wait to see how that
  // settles out before doing it that way.
  rocksdb::WriteBatchWithIndex batch(&kComparator, 0, true);
  rocksdb::WriteBatch b(ToString(repr));
  std::unique_ptr<rocksdb::WriteBatch::Handler> inserter(GetDBBatchInserter(&batch));
  rocksdb::Status status = b.Iterate(inserter.get());
  if (!status.ok()) {
    return ToDBStatus(status);
  }
  std::unique_ptr<rocksdb::Iterator> iter;
  iter.reset(batch.NewIteratorWithBase(rocksdb::NewEmptyIterator()));

  iter->SeekToFirst();
  if (iter->Valid() && kComparator.Compare(iter->key(), EncodeKey(start)) < 0) {
    return FmtStatus("key not in request range");
  }
  iter->SeekToLast();
  if (iter->Valid() && kComparator.Compare(iter->key(), EncodeKey(end)) >= 0) {
    return FmtStatus("key not in request range");
  }

  *stats = MVCCComputeStatsInternal(iter.get(), start, end, now_nanos);

  return kSuccess;
}
