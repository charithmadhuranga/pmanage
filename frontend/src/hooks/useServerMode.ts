import { useEffect, useState } from 'react';
import { Service as ServerMode } from '../../bindings/pmanage/pkg/servermode';

interface ServerInfo {
  enabled: boolean;
  address: string;
  auth: boolean;
  readOnly: boolean;
  remoteAdj: boolean;
}

export default function useServerMode() {
  const [info, setInfo] = useState<ServerInfo | null>(null);

  useEffect(() => {
    (ServerMode.Info() as unknown as Promise<ServerInfo>)
      .then(setInfo)
      .catch(() => setInfo(null));
  }, []);

  return info;
}